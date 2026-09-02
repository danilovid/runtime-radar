package tools

import (
	"context"
	"testing"
	"time"

	"github.com/cilium/tetragon/api/v1/tetragon"
	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/client"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Fixtures shared by the tests of this package.
const (
	testNamespace       = "prod"
	severityHigh        = "HIGH"
	detectorCryptominer = "CS_RT_CRYPTOMINER"
	detectorSuspShell   = "CS_RT_SUSP_SHELL"
)

// testNow is the fixed clock the tests run with, so that resolved time windows
// are comparable.
var testNow = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// mockRuntimeHistory records the requests it gets and replays canned answers.
type mockRuntimeHistory struct {
	history_api.RuntimeHistoryClient

	readResp   *processor_api.RuntimeEvent
	readErr    error
	filterResp []*history_api.ListRuntimeEventSliceResp
	filterErr  error

	readReqs   []*history_api.ReadRuntimeEventReq
	filterReqs []*history_api.FilterRuntimeEventSliceReq
}

func (m *mockRuntimeHistory) Read(_ context.Context, req *history_api.ReadRuntimeEventReq, _ ...grpc.CallOption) (*processor_api.RuntimeEvent, error) {
	m.readReqs = append(m.readReqs, req)

	return m.readResp, m.readErr
}

func (m *mockRuntimeHistory) FilterRuntimeEventSlice(_ context.Context, req *history_api.FilterRuntimeEventSliceReq, _ ...grpc.CallOption) (*history_api.ListRuntimeEventSliceResp, error) {
	m.filterReqs = append(m.filterReqs, req)

	if m.filterErr != nil {
		return nil, m.filterErr
	}

	if len(m.filterResp) == 0 {
		return &history_api.ListRuntimeEventSliceResp{}, nil
	}

	// Answers are replayed in order, the last one repeating: get_process_context
	// issues several filter calls per invocation.
	resp := m.filterResp[0]
	if len(m.filterResp) > 1 {
		m.filterResp = m.filterResp[1:]
	}

	return resp, nil
}

// mockRuntimeStats answers CountEvents from a per-type table.
type mockRuntimeStats struct {
	history_api.RuntimeStatsClient

	total  int32
	byType map[string]int32
	err    error

	reqs []*history_api.RuntimeEventsCounterReq
}

func (m *mockRuntimeStats) CountEvents(_ context.Context, req *history_api.RuntimeEventsCounterReq, _ ...grpc.CallOption) (*history_api.Counter, error) {
	m.reqs = append(m.reqs, req)

	if m.err != nil {
		return nil, m.err
	}

	if req.Type == nil {
		return &history_api.Counter{Count: m.total}, nil
	}

	return &history_api.Counter{Count: m.byType[req.GetType()]}, nil
}

// mockDetectors answers ListPage from a single page of detectors.
type mockDetectors struct {
	processor_api.DetectorControllerClient

	detectors []*processor_api.Detector
	err       error

	reqs []*processor_api.ListDetectorPageReq
}

func (m *mockDetectors) ListPage(_ context.Context, req *processor_api.ListDetectorPageReq, _ ...grpc.CallOption) (*processor_api.ListDetectorPageResp, error) {
	m.reqs = append(m.reqs, req)

	if m.err != nil {
		return nil, m.err
	}

	if req.GetPageNum() > 0 {
		return &processor_api.ListDetectorPageResp{Total: uint32(len(m.detectors))}, nil
	}

	return &processor_api.ListDetectorPageResp{Detectors: m.detectors, Total: uint32(len(m.detectors))}, nil
}

// newTestDeps builds Deps around the given mocks, with auth disabled and a
// fixed clock.
func newTestDeps(t *testing.T, history history_api.RuntimeHistoryClient, stats history_api.RuntimeStatsClient, detectors processor_api.DetectorControllerClient) *Deps {
	t.Helper()

	authorizer, err := auth.New(false, "")
	if err != nil {
		t.Fatalf("can't build authorizer: %v", err)
	}

	return &Deps{
		Clients: &client.Clients{
			RuntimeHistory: history,
			RuntimeStats:   stats,
			Detectors:      detectors,
		},
		Auth: authorizer,
		Now:  func() time.Time { return testNow },
	}
}

// execEvent builds a PROCESS_EXEC runtime event to compact.
func execEvent(id, namespace, pod, binary, arguments string, at time.Time) *processor_api.RuntimeEvent {
	return &processor_api.RuntimeEvent{
		Id:              id,
		TetragonVersion: "1.3.0",
		Event: &tetragon.GetEventsResponse{
			NodeName: "node-1",
			Time:     timestamppb.New(at),
			Event: &tetragon.GetEventsResponse_ProcessExec{
				ProcessExec: &tetragon.ProcessExec{
					Process: &tetragon.Process{
						ExecId:       "exec-" + id,
						ParentExecId: "parent-" + id,
						Binary:       binary,
						Arguments:    arguments,
						Cwd:          "/root",
						Pid:          wrapperspb.UInt32(42),
						Uid:          wrapperspb.UInt32(0),
						Pod: &tetragon.Pod{
							Namespace: namespace,
							Name:      pod,
							Workload:  "web",
							Container: &tetragon.Container{
								Name:  "app",
								Image: &tetragon.Image{Name: "registry.test/app:1.0"},
							},
						},
					},
					Parent: &tetragon.Process{
						ExecId: "parent-" + id,
						Binary: "/bin/bash",
					},
				},
			},
		},
	}
}
