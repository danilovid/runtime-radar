package model

import (
	"database/sql/driver"
	_ "embed"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
)

const (
	ConfigVersion Version = "1"
)

var (
	//go:embed policy/pod-exec.yaml
	podExec string

	//go:embed policy/privileged-containers.yaml
	privilegedContainers string

	//go:embed policy/host-namespaces.yaml
	hostNamespaces string

	//go:embed policy/host-path-volumes.yaml
	hostPathVolumes string

	//go:embed policy/cluster-admin-binding.yaml
	clusterAdminBinding string

	//go:embed policy/image-signature.yaml
	imageSignature string
)

var (
	DefaultConfig = &Config{
		Base: Base{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001")},
		Config: &ConfigJSON{
			Version: string(ConfigVersion),
			Policies: map[string]*api.KyvernoPolicy{
				"pod-exec": {
					Name:        "Running kubectl exec",
					Description: "This source tracks the pods/exec subresource allowing detection of arbitrary processes started in containers with kubectl exec. Unlike other sources, it produces no findings during a background scan, because exec is an action and not a state of a resource.",
					Yaml:        podExec,
					Enabled:     false,
					Action:      api.KyvernoPolicy_AUDIT,
					Severity:    "critical",
				},
				"privileged-containers": {
					Name:        "Privileged containers",
					Description: "This source tracks creation and update of pods running privileged containers or containers allowed to escalate their privileges. Pod controllers (deployments, statefulsets, daemonsets, replicasets, jobs, cronjobs) are covered as well.",
					Yaml:        privilegedContainers,
					Enabled:     false,
					Action:      api.KyvernoPolicy_AUDIT,
					Severity:    "high",
				},
				"host-namespaces": {
					Name:        "Sharing host namespaces",
					Description: "This source tracks pods sharing the host network, PID or IPC namespace. Such pods can observe and affect processes and traffic of the whole node, which effectively breaks container isolation.",
					Yaml:        hostNamespaces,
					Enabled:     false,
					Action:      api.KyvernoPolicy_AUDIT,
					Severity:    "high",
				},
				"host-path-volumes": {
					Name:        "Mounting host paths",
					Description: "This source tracks pods mounting a directory of the node into a container. Depending on the mounted path this allows reading node secrets or gaining full control over the node.",
					Yaml:        hostPathVolumes,
					Enabled:     false,
					Action:      api.KyvernoPolicy_AUDIT,
					Severity:    "high",
				},
				"cluster-admin-binding": {
					Name:        "Binding the cluster-admin role",
					Description: "This source tracks creation and update of role bindings referencing the built-in cluster-admin role, which is a common way of persisting after a cluster compromise.",
					Yaml:        clusterAdminBinding,
					Enabled:     false,
					Action:      api.KyvernoPolicy_AUDIT,
					Severity:    "medium",
				},
				"image-signature": {
					Name:        "Container image signature",
					Description: "This source verifies cosign signatures of container images with the public key specified in the manifest. To examine the source in detail and to set your own key, switch to expert mode. The source does nothing until the key is set.",
					Yaml:        imageSignature,
					Enabled:     false,
					Action:      api.KyvernoPolicy_AUDIT,
					Severity:    "high",
				},
			},
			HistoryControl: api.Config_ConfigJSON_WITH_THREATS,
		},
	}
)

type ConfigJSON api.Config_ConfigJSON

type Config struct {
	Base
	Config *ConfigJSON `gorm:"type:jsonb"`
}

// TableName method implements Tabler interface and makes GORM name the table of Config "admission_monitor_configs"
// instead of just "configs", the same way runtime-monitor does it.
func (Config) TableName() string {
	return "admission_monitor_configs"
}

func (s *ConfigJSON) Scan(src interface{}) error {
	b := src.([]byte)
	return json.Unmarshal(b, s)
}

func (s *ConfigJSON) Value() (driver.Value, error) {
	return json.Marshal(s)
}
