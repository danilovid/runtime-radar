package monitor

import (
	"testing"
	"time"

	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// defaultPolicyYAML is one of the manifests shipped with the product, it is used to make sure
// the tests stay in sync with what is actually applied to the cluster.
var defaultPolicyYAML = model.DefaultConfig.Config.Policies["privileged-containers"].GetYaml()

func testKyverno() *Kyverno {
	k := &Kyverno{Version: "v1.18.0"}
	k.SetConfig(&model.Config{
		Config: &model.ConfigJSON{
			Version: string(model.ConfigVersion),
			Policies: map[string]*api.KyvernoPolicy{
				"privileged-containers": {
					Name:     "Privileged containers",
					Yaml:     defaultPolicyYAML,
					Enabled:  true,
					Action:   api.KyvernoPolicy_AUDIT,
					Severity: "high",
				},
			},
		},
	})

	return k
}

func report(results ...map[string]interface{}) *unstructured.Unstructured {
	items := make([]interface{}, 0, len(results))
	for _, r := range results {
		items = append(items, r)
	}

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "wgpolicyk8s.io/v1alpha2",
		"kind":       "PolicyReport",
		"metadata":   map[string]interface{}{"name": "report", "namespace": "default"},
		"scope": map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap", // not a workload, so no attempt to enrich with containers
			"name":       "app",
			"namespace":  "default",
		},
		"results": items,
	}}
}

func newResult(policy, rule, res string, ts time.Time) map[string]interface{} {
	return map[string]interface{}{
		"policy":    policy,
		"rule":      rule,
		"result":    res,
		"message":   "validation error",
		"source":    "KyvernoValidatingPolicy",
		"timestamp": map[string]interface{}{"seconds": ts.Unix(), "nanos": int64(0)},
	}
}

func TestEventsFromReports(t *testing.T) {
	since := time.Now().Add(-time.Minute)
	now := time.Now()
	old := since.Add(-time.Hour)

	tests := []struct {
		name        string
		oldReport   *unstructured.Unstructured
		newReport   *unstructured.Unstructured
		wantThreats []string
	}{
		{
			name:        "new report with a failed result",
			newReport:   report(newResult("privileged-containers", "rule-0", resultFail, now)),
			wantThreats: []string{"privileged-containers/rule-0"},
		},
		{
			name:      "only results absent in the old report are reported",
			oldReport: report(newResult("privileged-containers", "rule-0", resultFail, now)),
			newReport: report(
				newResult("privileged-containers", "rule-0", resultFail, now),
				newResult("privileged-containers", "rule-1", resultFail, now),
			),
			wantThreats: []string{"privileged-containers/rule-1"},
		},
		{
			name:      "passed results are not threats",
			newReport: report(newResult("privileged-containers", "rule-0", "pass", now)),
		},
		{
			name:      "results produced before the monitor started are skipped",
			newReport: report(newResult("privileged-containers", "rule-0", resultFail, old)),
		},
		{
			name:      "policies managed outside of runtime radar are skipped",
			newReport: report(newResult("some-other-policy", "rule-0", resultFail, now)),
		},
	}

	k := testKyverno()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs := k.eventsFromReports(tt.oldReport, tt.newReport, since)

			if len(tt.wantThreats) == 0 {
				if len(evs) != 0 {
					t.Fatalf("expected no events, got %d", len(evs))
				}
				return
			}

			if len(evs) != 1 {
				t.Fatalf("expected 1 event, got %d", len(evs))
			}

			got := []string{}
			for _, threat := range evs[0].GetThreats() {
				got = append(got, threat.GetPolicy().GetId())

				if threat.GetSeverity() != "high" {
					t.Errorf("expected severity from the source, got '%s'", threat.GetSeverity())
				}
			}

			if len(got) != len(tt.wantThreats) {
				t.Fatalf("expected threats %v, got %v", tt.wantThreats, got)
			}
			for i, want := range tt.wantThreats {
				if got[i] != want {
					t.Errorf("expected threat '%s', got '%s'", want, got[i])
				}
			}
		})
	}
}

func TestDecodePolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  *api.KyvernoPolicy
		want    []string
		wantErr bool
	}{
		{
			name:   "audit action",
			policy: &api.KyvernoPolicy{Yaml: defaultPolicyYAML, Action: api.KyvernoPolicy_AUDIT},
			want:   []string{"Audit"},
		},
		{
			name:   "enforce action overwrites the manifest",
			policy: &api.KyvernoPolicy{Yaml: defaultPolicyYAML, Action: api.KyvernoPolicy_ENFORCE},
			want:   []string{"Deny"},
		},
		{
			name:    "unsupported kind",
			policy:  &api.KyvernoPolicy{Yaml: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app\n"},
			wantErr: true,
		},
		{
			name:    "no name",
			policy:  &api.KyvernoPolicy{Yaml: "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy\n"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, _, err := DecodePolicy("key", tt.policy)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, _, err := unstructured.NestedStringSlice(obj.Object, "spec", "validationActions")
			if err != nil {
				t.Fatalf("can't read validationActions: %v", err)
			}
			if len(got) != len(tt.want) || got[0] != tt.want[0] {
				t.Errorf("expected validationActions %v, got %v", tt.want, got)
			}

			if obj.GetLabels()[sourceLabel] != "key" {
				t.Errorf("expected source label to be set")
			}
		})
	}
}
