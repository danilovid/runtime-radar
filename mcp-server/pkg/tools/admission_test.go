package tools

import (
	"context"
	"testing"
	"time"

	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockAdmissionConfig replays one configuration and records what was saved.
type mockAdmissionConfig struct {
	monitor_api.ConfigControllerClient

	config  *monitor_api.Config
	readErr error

	saved []*monitor_api.Config
}

func (m *mockAdmissionConfig) Read(_ context.Context, _ *emptypb.Empty, _ ...grpc.CallOption) (*monitor_api.Config, error) {
	return m.config, m.readErr
}

func (m *mockAdmissionConfig) Add(_ context.Context, req *monitor_api.Config, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	m.saved = append(m.saved, req)

	return &emptypb.Empty{}, nil
}

// mockAdmissionHistory replays canned admission events.
type mockAdmissionHistory struct {
	history_api.AdmissionHistoryClient

	readResp   *monitor_api.AdmissionEvent
	filterResp *history_api.ListAdmissionEventSliceResp

	filterReqs []*history_api.FilterAdmissionEventSliceReq
}

func (m *mockAdmissionHistory) Read(_ context.Context, _ *history_api.ReadAdmissionEventReq, _ ...grpc.CallOption) (*monitor_api.AdmissionEvent, error) {
	return m.readResp, nil
}

func (m *mockAdmissionHistory) FilterAdmissionEventSlice(_ context.Context, req *history_api.FilterAdmissionEventSliceReq, _ ...grpc.CallOption) (*history_api.ListAdmissionEventSliceResp, error) {
	m.filterReqs = append(m.filterReqs, req)

	if m.filterResp == nil {
		return &history_api.ListAdmissionEventSliceResp{}, nil
	}

	return m.filterResp, nil
}

func admissionDeps(t *testing.T, config *mockAdmissionConfig, history *mockAdmissionHistory) *Deps {
	t.Helper()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.Clients.AdmissionConfig = config
	deps.Clients.AdmissionHistory = history

	return deps
}

func testAdmissionConfig() *monitor_api.Config {
	return &monitor_api.Config{
		Id: "00000000-0000-0000-0000-000000000001",
		Config: &monitor_api.Config_ConfigJSON{
			Version:        "1",
			HistoryControl: monitor_api.Config_ConfigJSON_WITH_THREATS,
			Policies: map[string]*monitor_api.KyvernoPolicy{
				"pod-exec": {
					Name:     "Running kubectl exec",
					Yaml:     "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy\n",
					Enabled:  true,
					Action:   monitor_api.KyvernoPolicy_ENFORCE,
					Severity: "critical",
				},
				"privileged-containers": {
					Name:     "Privileged containers",
					Yaml:     "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy\n",
					Enabled:  false,
					Action:   monitor_api.KyvernoPolicy_AUDIT,
					Severity: "high",
				},
			},
		},
	}
}

func TestListAdmissionSources(t *testing.T) {
	t.Parallel()

	deps := admissionDeps(t, &mockAdmissionConfig{config: testAdmissionConfig()}, &mockAdmissionHistory{})

	result, err := listAdmissionSources(deps)(context.Background(), ListAdmissionSourcesArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(result.Sources))
	}

	// The config is a map, so the answer has to come back in a stable order.
	if result.Sources[0].Key != "pod-exec" || result.Sources[1].Key != "privileged-containers" {
		t.Errorf("expected sources sorted by key, got %s then %s", result.Sources[0].Key, result.Sources[1].Key)
	}

	if result.EnabledCount != 1 || result.BlockingCount != 1 {
		t.Errorf("expected one enabled and one blocking source, got %d and %d", result.EnabledCount, result.BlockingCount)
	}

	if result.Sources[0].Action != admissionActionBlock {
		t.Errorf("expected the enforcing source to be reported as %s, got %s", admissionActionBlock, result.Sources[0].Action)
	}

	// Manifests are long and were not asked for.
	if result.Sources[0].Manifest != "" {
		t.Error("expected no manifest unless include_manifests is set")
	}
}

func TestSetAdmissionSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        SetAdmissionSourceArgs
		wantErr     bool
		wantEnabled bool
		wantAction  monitor_api.KyvernoPolicy_Action
	}{
		{
			name:        "enable a source",
			args:        SetAdmissionSourceArgs{Key: "privileged-containers", Enabled: boolPtr(true)},
			wantEnabled: true,
			wantAction:  monitor_api.KyvernoPolicy_AUDIT,
		},
		{
			name:        "switch a source to blocking",
			args:        SetAdmissionSourceArgs{Key: "privileged-containers", Action: admissionActionBlock},
			wantEnabled: false,
			wantAction:  monitor_api.KyvernoPolicy_ENFORCE,
		},
		{
			name:    "unknown source",
			args:    SetAdmissionSourceArgs{Key: "nope", Enabled: boolPtr(true)},
			wantErr: true,
		},
		{
			name:    "nothing to change",
			args:    SetAdmissionSourceArgs{Key: "privileged-containers"},
			wantErr: true,
		},
		{
			name:    "unknown action",
			args:    SetAdmissionSourceArgs{Key: "privileged-containers", Action: "warn"},
			wantErr: true,
		},
		{
			name:    "unknown severity",
			args:    SetAdmissionSourceArgs{Key: "privileged-containers", Severity: "spicy"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &mockAdmissionConfig{config: testAdmissionConfig()}
			deps := admissionDeps(t, config, &mockAdmissionHistory{})

			_, err := setAdmissionSource(deps)(context.Background(), tt.args)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got none")
				}
				if len(config.saved) != 0 {
					t.Error("expected nothing to be saved on a rejected change")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(config.saved) != 1 {
				t.Fatalf("expected the configuration to be saved once, got %d", len(config.saved))
			}

			saved := config.saved[0].GetConfig().GetPolicies()["privileged-containers"]
			if saved.GetEnabled() != tt.wantEnabled {
				t.Errorf("expected enabled=%v, got %v", tt.wantEnabled, saved.GetEnabled())
			}
			if saved.GetAction() != tt.wantAction {
				t.Errorf("expected action=%v, got %v", tt.wantAction, saved.GetAction())
			}

			// The other source must travel through untouched: the API takes the
			// whole configuration, so a change to one must not drop the rest.
			if other := config.saved[0].GetConfig().GetPolicies()["pod-exec"]; !other.GetEnabled() {
				t.Error("expected the other source to keep its state")
			}
		})
	}
}

func TestCreateAdmissionSource(t *testing.T) {
	t.Parallel()

	config := &mockAdmissionConfig{config: testAdmissionConfig()}
	deps := admissionDeps(t, config, &mockAdmissionHistory{})

	result, err := createAdmissionSource(deps)(context.Background(), CreateAdmissionSourceArgs{
		Key:         "require-non-root",
		Name:        "Run as non-root",
		Description: "Containers must not run as root",
		Manifest:    "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy\n",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A manifest a model produced is reviewed by a human before it can deny anything.
	if result.Source.Enabled {
		t.Error("expected a new source to be created switched off")
	}
	if result.Source.Action != admissionActionAudit {
		t.Errorf("expected a new source to start in %s, got %s", admissionActionAudit, result.Source.Action)
	}
	if result.Source.Severity != ruleSeverityMedium {
		t.Errorf("expected the default severity %s, got %s", ruleSeverityMedium, result.Source.Severity)
	}

	if len(config.saved) != 1 {
		t.Fatalf("expected the configuration to be saved once, got %d", len(config.saved))
	}
	if len(config.saved[0].GetConfig().GetPolicies()) != 3 {
		t.Error("expected the new source to be added next to the existing ones")
	}
}

func TestCreateAdmissionSourceRejectsDuplicate(t *testing.T) {
	t.Parallel()

	config := &mockAdmissionConfig{config: testAdmissionConfig()}
	deps := admissionDeps(t, config, &mockAdmissionHistory{})

	_, err := createAdmissionSource(deps)(context.Background(), CreateAdmissionSourceArgs{
		Key:         "pod-exec",
		Name:        "Another one",
		Description: "Clashes with an existing key",
		Manifest:    "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy\n",
	})
	if err == nil {
		t.Fatal("expected an error for a key that is already taken")
	}
	if len(config.saved) != 0 {
		t.Error("expected nothing to be saved")
	}
}

func TestSearchAdmissionEvents(t *testing.T) {
	t.Parallel()

	history := &mockAdmissionHistory{
		filterResp: &history_api.ListAdmissionEventSliceResp{
			AdmissionEvents: []*monitor_api.AdmissionEvent{testAdmissionEvent()},
		},
	}
	deps := admissionDeps(t, &mockAdmissionConfig{config: testAdmissionConfig()}, history)

	result, err := searchAdmissionEvents(deps)(context.Background(), SearchAdmissionEventsArgs{
		Namespace: []string{testNamespace},
		Blocked:   boolPtr(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 {
		t.Fatalf("expected 1 event, got %d", result.Count)
	}

	if got := result.Events[0].Policies; len(got) != 1 || got[0] != "privileged-containers/rule-0" {
		t.Errorf("expected the policy identifier in the summary, got %v", got)
	}

	if len(history.filterReqs) != 1 {
		t.Fatalf("expected one request, got %d", len(history.filterReqs))
	}

	filter := history.filterReqs[0].GetFilter()
	if filter.GetPeriod() == nil {
		t.Error("expected the period to always be set: history api rejects an empty filter")
	}
	if filter.GetBlocked() != true {
		t.Error("expected the blocked flag to reach the filter")
	}
	if result.Window.Limit != defaultSearchLimit {
		t.Errorf("expected the default limit %d, got %d", defaultSearchLimit, result.Window.Limit)
	}
}

func TestGetAdmissionEvent(t *testing.T) {
	t.Parallel()

	deps := admissionDeps(t,
		&mockAdmissionConfig{config: testAdmissionConfig()},
		&mockAdmissionHistory{readResp: testAdmissionEvent()},
	)

	detail, err := getAdmissionEvent(deps)(context.Background(), GetAdmissionEventArgs{ID: "event-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !detail.Blocked {
		t.Error("expected the event to be reported as blocked")
	}
	if len(detail.Threats) != 1 {
		t.Fatalf("expected 1 threat, got %d", len(detail.Threats))
	}
	if detail.Threats[0].Reason != "Privileged containers are not allowed." {
		t.Errorf("expected the reason Kyverno reported, got %q", detail.Threats[0].Reason)
	}

	if _, err := getAdmissionEvent(deps)(context.Background(), GetAdmissionEventArgs{}); err == nil {
		t.Error("expected an error for an empty id")
	}
}

func testAdmissionEvent() *monitor_api.AdmissionEvent {
	return &monitor_api.AdmissionEvent{
		Id:             "event-1",
		KyvernoVersion: "v1.18.0",
		RegisteredAt:   timestamppb.New(testNow.Add(-time.Hour)),
		Resource: &monitor_api.Resource{
			ApiVersion: "v1",
			Kind:       "Pod",
			Namespace:  testNamespace,
			Name:       "nginx",
		},
		Threats: []*monitor_api.Threat{{
			Policy: &monitor_api.Policy{
				Id:          "privileged-containers/rule-0",
				Name:        "Privileged containers",
				Rule:        "rule-0",
				Description: "Privileged containers are not allowed.",
			},
			Severity: "high",
		}},
		Blocked:          true,
		IsIncident:       true,
		IncidentSeverity: "high",
		BlockBy:          []string{"rule-id"},
	}
}

func boolPtr(v bool) *bool { return &v }
