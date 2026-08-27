package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/lib/docker"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	resultFail = "fail"
	resultWarn = "warn"

	// Enrichment happens in the informer handler, so it must not be able to stall the event stream.
	enrichTimeout = 5 * time.Second
)

// workloadGVRs maps kinds carrying a pod template to their resources. A policy report scoped to one
// of them is enriched with container images, so that image and registry scopes of policy-enforcer
// rules can be matched. Anything else is reported without containers.
var workloadGVRs = map[string]schema.GroupVersionResource{
	"Pod":                   {Group: "", Version: "v1", Resource: "pods"},
	"Deployment":            {Group: "apps", Version: "v1", Resource: "deployments"},
	"StatefulSet":           {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":             {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"ReplicaSet":            {Group: "apps", Version: "v1", Resource: "replicasets"},
	"ReplicationController": {Group: "", Version: "v1", Resource: "replicationcontrollers"},
	"Job":                   {Group: "batch", Version: "v1", Resource: "jobs"},
	"CronJob":               {Group: "batch", Version: "v1", Resource: "cronjobs"},
}

// result is a single entry of a policy report, see https://kyverno.io/docs/guides/reports/.
type result struct {
	policy    string
	rule      string
	result    string
	category  string
	message   string
	source    string
	timestamp time.Time
}

// key identifies the result within a report. Kyverno rewrites a report in place on every change,
// so results are compared by key to find out which of them are new.
func (r result) key() string {
	return r.policy + "/" + r.rule + "/" + r.result
}

// eventsFromReports converts results which are present in newReport but not in oldReport into admission
// events. oldReport is nil when the report has just been created. Results older than since are skipped:
// they were produced before this monitor started and have already been reported by the previous run.
func (k *Kyverno) eventsFromReports(oldReport, newReport *unstructured.Unstructured, since time.Time) []*api.AdmissionEvent {
	seen := make(map[string]bool)
	for _, r := range resultsFromReport(oldReport) {
		seen[r.key()] = true
	}

	policies := k.policiesByName()

	var threats []*api.Threat
	for _, r := range resultsFromReport(newReport) {
		if r.result != resultFail && r.result != resultWarn {
			continue
		}
		if seen[r.key()] || r.timestamp.Before(since) {
			continue
		}

		// Kyverno may be shared with policies managed outside of Runtime Radar, they are not our business.
		p, ok := policies[r.policy]
		if !ok {
			continue
		}

		threats = append(threats, &api.Threat{
			Policy: &api.Policy{
				Id:          r.policy + "/" + r.rule,
				Name:        p.GetName(),
				Rule:        r.rule,
				Category:    r.category,
				Description: r.message,
				Source:      r.source,
				Action:      p.GetAction(),
			},
			Severity: p.GetSeverity(),
		})
	}

	if len(threats) == 0 {
		return nil
	}

	res := k.resourceFromReport(newReport)

	return []*api.AdmissionEvent{{
		Id:             uuid.NewString(),
		KyvernoVersion: k.Version,
		RegisteredAt:   timestamppb.Now(),
		Resource:       res,
		Threats:        threats,
		// Kyverno records blocked requests in Kubernetes events only, a report is always about a
		// resource which exists, see https://kyverno.io/docs/guides/reports/.
		Blocked: false,
	}}
}

// policiesByName indexes sources by the name of the policy they create in cluster. The name is taken
// from the manifest and may differ from the source key when the manifest was edited in expert mode.
func (k *Kyverno) policiesByName() map[string]*api.KyvernoPolicy {
	cfg := k.Config()
	res := make(map[string]*api.KyvernoPolicy, len(cfg.Config.Policies))

	for key, p := range cfg.Config.Policies {
		obj, _, err := DecodePolicy(key, p)
		if err != nil {
			log.Error().Err(err).Msgf("Can't decode policy '%s'", key)
			continue
		}

		res[obj.GetName()] = p
	}

	return res
}

func resultsFromReport(report *unstructured.Unstructured) []result {
	if report == nil {
		return nil
	}

	raw, found, err := unstructured.NestedSlice(report.Object, "results")
	if err != nil || !found {
		return nil
	}

	res := make([]result, 0, len(raw))

	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		r := result{
			policy:   nestedString(m, "policy"),
			rule:     nestedString(m, "rule"),
			result:   nestedString(m, "result"),
			category: nestedString(m, "category"),
			message:  nestedString(m, "message"),
			source:   nestedString(m, "source"),
		}

		// Timestamp is metav1.Timestamp, that is seconds and nanos, and not the RFC3339 string
		// used everywhere else in the Kubernetes API.
		if seconds, found, _ := unstructured.NestedInt64(m, "timestamp", "seconds"); found {
			nanos, _, _ := unstructured.NestedInt64(m, "timestamp", "nanos")
			r.timestamp = time.Unix(seconds, nanos)
		}

		res = append(res, r)
	}

	return res
}

// resourceFromReport builds the resource the report is scoped to and, when the resource carries a pod
// template, enriches it with containers. Enrichment is best effort: the resource may already be gone.
func (k *Kyverno) resourceFromReport(report *unstructured.Unstructured) *api.Resource {
	scope, found, err := unstructured.NestedMap(report.Object, "scope")
	if err != nil || !found {
		return &api.Resource{Namespace: report.GetNamespace()}
	}

	res := &api.Resource{
		ApiVersion: nestedString(scope, "apiVersion"),
		Kind:       nestedString(scope, "kind"),
		Namespace:  nestedString(scope, "namespace"),
		Name:       nestedString(scope, "name"),
		Uid:        nestedString(scope, "uid"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), enrichTimeout)
	defer cancel()

	obj, err := k.getResource(ctx, res)
	if err != nil {
		log.Debug().Err(err).Str("kind", res.Kind).Str("name", res.Name).Msgf("Can't enrich resource with containers")
		return res
	}
	if obj == nil {
		return res
	}

	res.NodeName = nestedString(obj.Object, "spec", "nodeName")
	res.Containers = containersFromObject(obj)

	return res
}

func (k *Kyverno) getResource(ctx context.Context, res *api.Resource) (*unstructured.Unstructured, error) {
	gvr, ok := workloadGVRs[res.GetKind()]
	if !ok {
		return nil, nil
	}

	obj, err := k.dynamicClient.Resource(gvr).Namespace(res.GetNamespace()).Get(ctx, res.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("can't get %s '%s/%s': %w", res.GetKind(), res.GetNamespace(), res.GetName(), err)
	}

	return obj, nil
}

// podSpecPaths are the places a pod spec can be found at, depending on the kind of the resource.
var podSpecPaths = [][]string{
	{"spec"},                     // Pod
	{"spec", "template", "spec"}, // Deployment, StatefulSet, DaemonSet, ReplicaSet, ReplicationController, Job
	{"spec", "jobTemplate", "spec", "template", "spec"}, // CronJob
}

func containersFromObject(obj *unstructured.Unstructured) []*api.Container {
	var podSpec map[string]interface{}

	for _, path := range podSpecPaths {
		m, found, err := unstructured.NestedMap(obj.Object, path...)
		if err != nil || !found {
			continue
		}
		if _, hasContainers := m["containers"]; hasContainers {
			podSpec = m
			break
		}
	}

	if podSpec == nil {
		return nil
	}

	var res []*api.Container

	for _, field := range []string{"containers", "initContainers", "ephemeralContainers"} {
		raw, found, err := unstructured.NestedSlice(podSpec, field)
		if err != nil || !found {
			continue
		}

		for _, item := range raw {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			c := &api.Container{Name: nestedString(m, "name")}

			if image := nestedString(m, "image"); image != "" {
				ref, err := docker.ParseReference(image)
				if err != nil {
					log.Debug().Err(err).Str("image", image).Msgf("Can't parse image reference")
					c.ImageName = image
				} else {
					c.ImageName, c.Registry = ref.Image, ref.Registry
				}
			}

			res = append(res, c)
		}
	}

	return res
}

func nestedString(obj map[string]interface{}, fields ...string) string {
	s, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return ""
	}

	return strings.TrimSpace(s)
}
