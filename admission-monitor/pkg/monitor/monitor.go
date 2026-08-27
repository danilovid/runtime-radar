package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/build"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/model"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/monitor/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/yaml"
)

const (
	// Label put on every policy applied by admission-monitor. Policies without it are never
	// touched, so that Kyverno can be shared with policies managed outside of Runtime Radar.
	managedByLabel = "app.kubernetes.io/managed-by"
	// Label keeping the key of the source the policy was created from.
	sourceLabel = "admission-monitor.runtime-radar.io/source"
	// Label of the Kyverno admission controller deployment carrying its version.
	versionLabel = "app.kubernetes.io/version"

	// Name of the Kyverno admission controller deployment, used to detect Kyverno version.
	kyvernoDeployment = "kyverno-admission-controller"

	// Kyverno keeps policy reports in sync with the cluster on its own, so there is nothing
	// we could gain from resyncing informers.
	informerResync = 0

	dispatchTimeout = time.Second
)

var (
	// policyGVRs maps supported Kyverno policy kinds to their resources. Only these kinds can be
	// used as a source, everything else is rejected by the config service.
	policyGVRs = map[string]schema.GroupVersionResource{
		"ValidatingPolicy":      {Group: "policies.kyverno.io", Version: "v1", Resource: "validatingpolicies"},
		"ImageValidatingPolicy": {Group: "policies.kyverno.io", Version: "v1", Resource: "imagevalidatingpolicies"},
	}

	policyReportGVR        = schema.GroupVersionResource{Group: "wgpolicyk8s.io", Version: "v1alpha2", Resource: "policyreports"}
	clusterPolicyReportGVR = schema.GroupVersionResource{Group: "wgpolicyk8s.io", Version: "v1alpha2", Resource: "clusterpolicyreports"}
)

// Monitor is interface of Kyverno monitoring instance.
type Monitor interface {
	Config() *model.Config
	SetConfig(cfg *model.Config)
	Init(ctx context.Context, cfg *model.Config) error
	Reinit(sel config.Selector, cfg *model.Config)
	Run(stop <-chan struct{}) error
	Events() <-chan *api.AdmissionEvent
}

// Kyverno is implementation of Monitor. Unlike Tetragon, Kyverno has no streaming API: it reports
// policy results as PolicyReport/ClusterPolicyReport resources, so the event stream is an informer
// over these two resources.
type Kyverno struct {
	Version string

	dynamicClient dynamic.Interface
	// namespace Kyverno itself is installed in, its own resources are never reported.
	namespace string

	config   *model.Config
	configMu sync.RWMutex

	ready  chan struct{}
	reinit chan config.InitKyverno
	events chan *api.AdmissionEvent
}

// NewKyverno creates new Kyverno instance. It returns any possible error and closing function which is
// supposed to be put in defer statement in main.
func NewKyverno(namespace string, bufferSize int) (*Kyverno, func() error, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("can't get in-cluster config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("can't create dynamic client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("can't create k8s client: %w", err)
	}

	version, err := kyvernoVersion(context.Background(), clientset, namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("can't get kyverno version: %w", err)
	}

	k := &Kyverno{
		Version:       version,
		dynamicClient: dynamicClient,
		namespace:     namespace,

		ready:  make(chan struct{}),
		reinit: make(chan config.InitKyverno),
		events: make(chan *api.AdmissionEvent, bufferSize),
	}

	// Dynamic client holds no connection of its own, closing is a no-op kept for symmetry with other components.
	return k, func() error { return nil }, nil
}

// Config returns current Kyverno config. It's safe for concurrent use.
func (k *Kyverno) Config() *model.Config {
	k.configMu.RLock()
	defer k.configMu.RUnlock()

	return k.config
}

// SetConfig sets new Kyverno config (but does not apply it). It's safe for concurrent use.
func (k *Kyverno) SetConfig(cfg *model.Config) {
	k.configMu.Lock()
	defer k.configMu.Unlock()

	k.config = cfg
}

// Events returns read-only channel for consuming admission events elsewhere.
func (k *Kyverno) Events() <-chan *api.AdmissionEvent {
	return k.events
}

// Reinit reinitializes Kyverno based on config.Selector and given config.
func (k *Kyverno) Reinit(sel config.Selector, cfg *model.Config) {
	k.reinit <- config.InitKyverno{
		Selector: sel,
		Config:   cfg,
	}
}

// Init should be run on new Kyverno instance, in order to initialize and prepare it for Run.
func (k *Kyverno) Init(ctx context.Context, cfg *model.Config) error {
	defer close(k.ready)

	return k.initBySelector(ctx, config.Selector{Policies: true}, cfg)
}

func (k *Kyverno) initBySelector(ctx context.Context, c config.Selector, cfg *model.Config) error {
	k.configMu.Lock()
	defer k.configMu.Unlock()

	if c.Policies {
		if err := k.initPolicies(ctx, cfg); err != nil {
			return err
		}
	}

	k.config = cfg

	return nil
}

// initPolicies brings the set of Kyverno policies in cluster in sync with the config.
//
// Just like runtime-monitor does with Tetragon tracing policies, re-initialization works in two steps:
// 1. Delete policies previously applied by admission-monitor
// 2. Apply policies enabled in the config
//
// Unlike Tetragon, Kyverno has no notion of a disabled policy, so a disabled source simply has no
// policy in cluster. Policies without our managed-by label are never touched.
func (k *Kyverno) initPolicies(ctx context.Context, cfg *model.Config) error {
	selector := labels.Set{managedByLabel: build.AppName}.String()

	for kind, gvr := range policyGVRs {
		list, err := k.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return fmt.Errorf("can't list %s: %w", kind, err)
		}

		for _, p := range list.Items {
			if err := k.dynamicClient.Resource(gvr).Delete(ctx, p.GetName(), metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("can't delete %s '%s': %w", kind, p.GetName(), err)
			}
			log.Info().Msgf("Kyverno policy '%s/%s' deleted", kind, p.GetName())
		}
	}

	for key, p := range cfg.Config.Policies {
		if !p.GetEnabled() {
			continue
		}

		obj, gvr, err := DecodePolicy(key, p)
		if err != nil {
			return fmt.Errorf("can't decode policy '%s': %w", key, err)
		}

		if _, err := k.dynamicClient.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("can't create policy '%s': %w", key, err)
		}
		log.Info().Str("policy", p.GetYaml()).Msgf("Kyverno policy '%s/%s' applied", obj.GetKind(), obj.GetName())
	}

	return nil
}

// DecodePolicy converts a source into an unstructured Kyverno policy ready to be applied.
// It's exported because the config service uses it to validate a manifest before storing it.
func DecodePolicy(key string, p *api.KyvernoPolicy) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(p.GetYaml()), &raw); err != nil {
		return nil, schema.GroupVersionResource{}, fmt.Errorf("can't parse manifest: %w", err)
	}

	obj := &unstructured.Unstructured{Object: raw}

	gvr, ok := policyGVRs[obj.GetKind()]
	if !ok {
		return nil, schema.GroupVersionResource{}, fmt.Errorf("unsupported policy kind '%s'", obj.GetKind())
	}
	if obj.GetName() == "" {
		return nil, schema.GroupVersionResource{}, fmt.Errorf("policy has no name")
	}

	ls := obj.GetLabels()
	if ls == nil {
		ls = map[string]string{}
	}
	ls[managedByLabel] = build.AppName
	ls[sourceLabel] = key
	obj.SetLabels(ls)

	// The action is a property of the source and not of the manifest, so that switching a source
	// from audit to enforce never requires editing YAML in expert mode.
	if err := unstructured.SetNestedStringSlice(obj.Object, validationActions(p.GetAction()), "spec", "validationActions"); err != nil {
		return nil, schema.GroupVersionResource{}, fmt.Errorf("can't set validationActions: %w", err)
	}

	return obj, gvr, nil
}

func validationActions(a api.KyvernoPolicy_Action) []string {
	if a == api.KyvernoPolicy_ENFORCE {
		return []string{"Deny"}
	}
	return []string{"Audit"}
}

// Run runs the monitor. Init should be invoked before Run.
func (k *Kyverno) Run(stop <-chan struct{}) error {
	log.Info().Msgf("Kyverno monitor started")
	defer log.Info().Msgf("Kyverno monitor stopped")

	<-k.ready // <-- wait for ready status

	defer close(k.events)

	informers := dynamicinformer.NewFilteredDynamicSharedInformerFactory(k.dynamicClient, informerResync, metav1.NamespaceAll, nil)

	// Results already present in the cluster when the monitor starts belong to the previous run and
	// were reported by it, so only results produced from now on are dispatched.
	since := time.Now()

	for _, gvr := range []schema.GroupVersionResource{policyReportGVR, clusterPolicyReportGVR} {
		i := informers.ForResource(gvr).Informer()

		if _, err := i.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				k.dispatchResults(nil, obj, since)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				k.dispatchResults(oldObj, newObj, since)
			},
		}); err != nil {
			return fmt.Errorf("can't add event handler for %s: %w", gvr.Resource, err)
		}
	}

	informerStop := make(chan struct{})
	defer close(informerStop)

	informers.Start(informerStop)
	informers.WaitForCacheSync(informerStop)

	for {
		select {
		case it := <-k.reinit:
			if err := k.initBySelector(context.Background(), it.Selector, it.Config); err != nil {
				log.Error().Err(err).Interface("selector", it.Selector).Interface("config", it.Config).Msgf("Can't init kyverno")
				// Just log, don't break the monitor loop
			}
		case <-stop:
			return nil // <-- return
		}
	}
}

// dispatchResults converts results which appeared in newObj but were not in oldObj into admission
// events and puts them into the events channel. oldObj is nil when the report has just been created.
func (k *Kyverno) dispatchResults(oldObj, newObj interface{}, since time.Time) {
	newReport, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		log.Error().Interface("object", newObj).Msgf("Got unexpected object instead of policy report")
		return
	}

	if newReport.GetNamespace() == k.namespace {
		return
	}

	var oldReport *unstructured.Unstructured
	if oldObj != nil {
		if oldReport, ok = oldObj.(*unstructured.Unstructured); !ok {
			log.Error().Interface("object", oldObj).Msgf("Got unexpected object instead of policy report")
			return
		}
	}

	for _, ev := range k.eventsFromReports(oldReport, newReport, since) {
		timer := time.NewTimer(dispatchTimeout)

		select {
		case k.events <- ev:
		case <-timer.C:
			log.Error().Interface("event", ev).Msgf("Timeout dispatching event")
		}

		timer.Stop()
	}
}

// kyvernoVersion reads the version of the installed Kyverno from the label of its admission controller
// deployment. Apart from being reported with every event, it is the earliest check that Kyverno is
// actually installed in the cluster.
func kyvernoVersion(ctx context.Context, clientset kubernetes.Interface, namespace string) (string, error) {
	d, err := clientset.AppsV1().Deployments(namespace).Get(ctx, kyvernoDeployment, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("can't get '%s/%s' deployment: %w", namespace, kyvernoDeployment, err)
	}

	return d.GetLabels()[versionLabel], nil
}
