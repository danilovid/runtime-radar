package monitor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/runtime-radar/runtime-radar/runtime-monitor/api"
	"github.com/runtime-radar/runtime-radar/runtime-monitor/pkg/model"
	"google.golang.org/grpc"
)

// sensorsStub answers only the calls initTracingPolicies makes, refusing the
// policies named in reject.
type sensorsStub struct {
	tetragon.FineGuidanceSensorsClient

	reject map[string]bool
	added  []string
}

func (s *sensorsStub) ListTracingPolicies(
	context.Context, *tetragon.ListTracingPoliciesRequest, ...grpc.CallOption,
) (*tetragon.ListTracingPoliciesResponse, error) {
	return &tetragon.ListTracingPoliciesResponse{}, nil
}

func (s *sensorsStub) AddTracingPolicy(
	_ context.Context, req *tetragon.AddTracingPolicyRequest, _ ...grpc.CallOption,
) (*tetragon.AddTracingPolicyResponse, error) {
	// The stub keys on the policy name embedded in the YAML the config holds.
	name := strings.TrimSpace(req.GetYaml())
	if s.reject[name] {
		return nil, fmt.Errorf("validation failed: call %q not found", name)
	}

	s.added = append(s.added, name)

	return &tetragon.AddTracingPolicyResponse{}, nil
}

func configWith(names ...string) *model.Config {
	policies := map[string]*api.TracingPolicy{}
	for _, name := range names {
		policies[name] = &api.TracingPolicy{Yaml: name}
	}

	return &model.Config{Config: &model.ConfigJSON{TracingPolicies: policies}}
}

// A kernel without the symbol one policy hooks costs that policy, not the
// whole monitor: the others still load and the service still starts.
func TestInitTracingPoliciesSurvivesOneRejection(t *testing.T) {
	stub := &sensorsStub{reject: map[string]bool{"kernel-modules": true}}
	tetra := &Tetra{sensorsClient: stub}

	if err := tetra.initTracingPolicies(context.Background(), configWith("kernel-modules", "connect", "mount")); err != nil {
		t.Fatalf("one rejected policy must not fail initialisation: %v", err)
	}

	if len(stub.added) != 2 {
		t.Errorf("added %v, want the two policies Tetragon accepted", stub.added)
	}
}

// Everything being rejected is not a kernel quirk but a broken Tetragon, and
// starting then would leave the monitor watching nothing while looking healthy.
func TestInitTracingPoliciesFailsWhenNothingLoads(t *testing.T) {
	stub := &sensorsStub{reject: map[string]bool{"connect": true, "mount": true}}
	tetra := &Tetra{sensorsClient: stub}

	err := tetra.initTracingPolicies(context.Background(), configWith("connect", "mount"))
	if err == nil {
		t.Fatal("initialisation must fail when no policy could be loaded")
	}

	if !strings.Contains(err.Error(), "every tracing policy") {
		t.Errorf("unexpected error: %v", err)
	}
}
