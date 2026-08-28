package tools

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// directionRight asks History API for the events preceding the cursor,
	// newest first, which is what "the last N events" means.
	directionRight = "right"

	// defaultSearchLimit and maxSearchLimit bound how many events a single
	// search returns. The ceiling is what keeps a compact summary compact.
	defaultSearchLimit = 20
	maxSearchLimit     = 50

	// defaultContextLimit is both the default and the ceiling for the number of
	// events describing one process.
	defaultContextLimit = 20

	// defaultLookback is the window a search covers when the caller gives no
	// lower bound. It also guarantees the filter is never empty, which History
	// API rejects.
	defaultLookback = 24 * time.Hour
)

// SearchRuntimeEventsArgs are the arguments of search_runtime_events.
type SearchRuntimeEventsArgs struct {
	Namespace   []string `json:"namespace,omitempty" jsonschema:"Kubernetes namespaces to search in. Supports globs, for example prod-*"`
	Pod         []string `json:"pod,omitempty" jsonschema:"pod names to search in. Supports globs, for example nginx-*"`
	Binary      []string `json:"binary,omitempty" jsonschema:"absolute paths of executed binaries. Supports globs, for example /usr/bin/*"`
	HasThreats  *bool    `json:"has_threats,omitempty" jsonschema:"true returns only events a detector reported a threat on, false only events without threats. Omit to return both"`
	DetectorIDs []string `json:"detector_ids,omitempty" jsonschema:"identifiers of the detectors that reported a threat, for example CS_RT_CRYPTOMINER. Exact match, no globs. Call list_detectors to discover them"`
	Since       string   `json:"since,omitempty" jsonschema:"start of the time window, RFC3339. Defaults to 24 hours before now"`
	Until       string   `json:"until,omitempty" jsonschema:"end of the time window, RFC3339. Defaults to now"`
	Limit       int      `json:"limit,omitempty" jsonschema:"maximum number of events to return, 1 to 50. Defaults to 20, larger values are capped at 50"`
}

// SearchWindow reports the time window and size a search was actually run with,
// so that a caller relying on defaults knows what it got.
type SearchWindow struct {
	Since string `json:"since" jsonschema:"start of the window the search covered, RFC3339"`
	Until string `json:"until" jsonschema:"end of the window the search covered, RFC3339"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of events the search was allowed to return"`
}

// SearchRuntimeEventsResult is the compact answer of search_runtime_events.
type SearchRuntimeEventsResult struct {
	Events  []EventSummary `json:"events" jsonschema:"matching events, newest first"`
	Count   int            `json:"count" jsonschema:"number of events returned"`
	HasMore bool           `json:"has_more" jsonschema:"true when the limit was reached and older matching events may exist. Narrow the window or the filter to see them"`
	Window  SearchWindow   `json:"window" jsonschema:"the time window and limit the search was run with"`
}

// GetRuntimeEventArgs are the arguments of get_runtime_event.
type GetRuntimeEventArgs struct {
	ID string `json:"id" jsonschema:"identifier of the runtime event, as returned by search_runtime_events"`
}

// GetProcessContextArgs are the arguments of get_process_context.
type GetProcessContextArgs struct {
	EventID string `json:"event_id" jsonschema:"identifier of the runtime event to build the process context around"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum number of events to return, 1 to 20. Defaults to 20"`
}

// ProcessContextResult is the answer of get_process_context: what the process
// of an event did, what its parent did, and what it spawned.
type ProcessContextResult struct {
	EventID      string         `json:"event_id" jsonschema:"the event the context was built around"`
	ExecID       string         `json:"process_exec_id,omitempty" jsonschema:"execution identifier of the process of that event"`
	ParentExecID string         `json:"process_parent_exec_id,omitempty" jsonschema:"execution identifier of its parent process"`
	Process      *ProcessInfo   `json:"process,omitempty" jsonschema:"the process of that event"`
	Parent       *ProcessInfo   `json:"parent,omitempty" jsonschema:"the parent of that process"`
	Events       []EventSummary `json:"events" jsonschema:"events of the same process, of its parent and of its children, oldest first"`
	Count        int            `json:"count" jsonschema:"number of events returned"`
}

func registerEventTools(server *mcp.Server, deps *Deps) {
	events := []auth.Permission{auth.ReadEvents()}

	addScopedTool(server, deps, auth.ScopeRuntimeMonitor, &mcp.Tool{
		Name:        "search_runtime_events",
		Annotations: readOnly("Search runtime events"),
		Description: "Search the runtime events Runtime Radar recorded from Kubernetes workloads, filtered by " +
			"namespace, pod, binary, detector or time window. Returns a compact summary of each event " +
			"(identity, time, type, pod, container, image, binary and arguments, detected threats) rather than the " +
			"raw Tetragon payload; use get_runtime_event for the full record of one event. Start here to answer " +
			"questions like \"what suspicious activity happened in namespace X\".",
	}, events, searchRuntimeEvents(deps))

	addScopedTool(server, deps, auth.ScopeRuntimeMonitor, &mcp.Tool{
		Name:        "get_runtime_event",
		Annotations: readOnly("Get runtime event"),
		Description: "Read one runtime event by its identifier: the full record, including the process and its " +
			"parent, the probe arguments, stack traces, the threats detectors reported and the detectors that " +
			"failed to run on it. Long arguments and stack traces are truncated.",
	}, events, getRuntimeEvent(deps))

	addScopedTool(server, deps, auth.ScopeRuntimeMonitor, &mcp.Tool{
		Name:        "get_process_context",
		Annotations: readOnly("Get process context"),
		Description: "Given one runtime event, return what else the same process did, what its parent process did " +
			"and what that process spawned, oldest first. Use it to tell a one-off command apart from a chain of " +
			"actions after an event looks suspicious.",
	}, events, getProcessContext(deps))
}

func searchRuntimeEvents(deps *Deps) func(context.Context, SearchRuntimeEventsArgs) (SearchRuntimeEventsResult, error) {
	return func(ctx context.Context, args SearchRuntimeEventsArgs) (SearchRuntimeEventsResult, error) {
		req, window, err := args.request(deps.now())
		if err != nil {
			return SearchRuntimeEventsResult{}, err
		}

		resp, err := deps.Clients.RuntimeHistory.FilterRuntimeEventSlice(ctx, req)
		if err != nil {
			return SearchRuntimeEventsResult{}, fmt.Errorf("can't search runtime events: %w", err)
		}

		events := resp.GetRuntimeEvents()
		summaries := make([]EventSummary, 0, len(events))
		for _, event := range events {
			summaries = append(summaries, summarize(event))
		}

		return SearchRuntimeEventsResult{
			Events:  summaries,
			Count:   len(summaries),
			HasMore: len(summaries) >= window.Limit,
			Window:  window,
		}, nil
	}
}

// request converts the tool arguments into a History API filter request and
// reports the window it resolved defaults to.
func (a SearchRuntimeEventsArgs) request(now time.Time) (*history_api.FilterRuntimeEventSliceReq, SearchWindow, error) {
	limit := a.Limit
	switch {
	case limit <= 0:
		limit = defaultSearchLimit
	case limit > maxSearchLimit:
		limit = maxSearchLimit
	}

	until, err := parseTime(a.Until, now)
	if err != nil {
		return nil, SearchWindow{}, status.Errorf(codes.InvalidArgument, "can't parse until: %v", err)
	}

	since, err := parseTime(a.Since, until.Add(-defaultLookback))
	if err != nil {
		return nil, SearchWindow{}, status.Errorf(codes.InvalidArgument, "can't parse since: %v", err)
	}

	if !since.Before(until) {
		return nil, SearchWindow{}, status.Error(codes.InvalidArgument, "since must be earlier than until")
	}

	filter := &history_api.RuntimeFilter{
		ProcessPodNamespace: a.Namespace,
		ProcessPodName:      a.Pod,
		ProcessBinary:       a.Binary,
		ThreatsDetectors:    a.DetectorIDs,
		HasThreats:          a.HasThreats,
		// The period is always set: it bounds the scan, and History API rejects
		// a filter with no criterion at all.
		Period: &history_api.Period{From: timestamppb.New(since), To: timestamppb.New(until)},
	}

	req := &history_api.FilterRuntimeEventSliceReq{
		// The cursor is exclusive and the direction makes History API walk
		// backwards from it, so the newest events of the window come first.
		Cursor:    timestamppb.New(until),
		Direction: directionRight,
		SliceSize: uint32(limit), // #nosec G115 -- limit is bounded by maxSearchLimit above
		Filter:    filter,
	}

	window := SearchWindow{
		Since: since.Format(time.RFC3339),
		Until: until.Format(time.RFC3339),
		Limit: limit,
	}

	return req, window, nil
}

func getRuntimeEvent(deps *Deps) func(context.Context, GetRuntimeEventArgs) (EventDetail, error) {
	return func(ctx context.Context, args GetRuntimeEventArgs) (EventDetail, error) {
		if args.ID == "" {
			return EventDetail{}, status.Error(codes.InvalidArgument, "id is empty")
		}

		event, err := deps.Clients.RuntimeHistory.Read(ctx, &history_api.ReadRuntimeEventReq{Id: args.ID})
		if err != nil {
			return EventDetail{}, fmt.Errorf("can't read runtime event: %w", err)
		}

		return detail(event), nil
	}
}

func getProcessContext(deps *Deps) func(context.Context, GetProcessContextArgs) (ProcessContextResult, error) {
	return func(ctx context.Context, args GetProcessContextArgs) (ProcessContextResult, error) {
		if args.EventID == "" {
			return ProcessContextResult{}, status.Error(codes.InvalidArgument, "event_id is empty")
		}

		limit := args.Limit
		if limit <= 0 || limit > defaultContextLimit {
			limit = defaultContextLimit
		}

		anchor, err := deps.Clients.RuntimeHistory.Read(ctx, &history_api.ReadRuntimeEventReq{Id: args.EventID})
		if err != nil {
			return ProcessContextResult{}, fmt.Errorf("can't read runtime event: %w", err)
		}

		process, parent := processes(anchor.GetEvent())
		execID := process.GetExecId()
		parentExecID := process.GetParentExecId()

		result := ProcessContextResult{
			EventID:      anchor.GetId(),
			ExecID:       execID,
			ParentExecID: parentExecID,
			Process:      processInfo(process),
			Parent:       processInfo(parent),
			Events:       []EventSummary{},
		}

		if execID == "" {
			// Nothing to correlate on: report the event itself and say so by
			// returning a single-event context rather than an error.
			result.Events = append(result.Events, summarize(anchor))
			result.Count = len(result.Events)

			return result, nil
		}

		// Three questions, three filters: what this process did, what its
		// parent did, and what this process spawned.
		filters := []*history_api.RuntimeFilter{
			{ProcessExecId: execID},
			{ProcessParentExecId: execID},
		}
		if parentExecID != "" {
			filters = append(filters, &history_api.RuntimeFilter{ProcessExecId: parentExecID})
		}

		seen := make(map[string]struct{}, limit)
		collected := make([]*eventWithTime, 0, limit)

		for _, filter := range filters {
			events, err := deps.slice(ctx, filter, limit)
			if err != nil {
				return ProcessContextResult{}, err
			}

			for _, event := range events {
				if _, ok := seen[event.GetId()]; ok {
					continue
				}
				seen[event.GetId()] = struct{}{}

				collected = append(collected, &eventWithTime{
					summary: summarize(event),
					at:      event.GetEvent().GetTime().AsTime(),
				})
			}
		}

		// Each filter returns its own newest events, so more than the limit may
		// have been collected. Keep the ones closest in time to the event the
		// context was asked about — that event included — rather than an
		// arbitrary end of the range.
		if len(collected) > limit {
			anchorAt := anchor.GetEvent().GetTime().AsTime()

			sort.SliceStable(collected, func(i, j int) bool {
				return absDuration(collected[i].at.Sub(anchorAt)) < absDuration(collected[j].at.Sub(anchorAt))
			})

			collected = collected[:limit]
		}

		sort.SliceStable(collected, func(i, j int) bool {
			return collected[i].at.Before(collected[j].at)
		})

		for _, event := range collected {
			result.Events = append(result.Events, event.summary)
		}
		result.Count = len(result.Events)

		return result, nil
	}
}

type eventWithTime struct {
	summary EventSummary
	at      time.Time
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
}

// slice reads the newest events matching filter.
func (d *Deps) slice(ctx context.Context, filter *history_api.RuntimeFilter, limit int) ([]*processor_api.RuntimeEvent, error) {
	resp, err := d.Clients.RuntimeHistory.FilterRuntimeEventSlice(ctx, &history_api.FilterRuntimeEventSliceReq{
		Cursor:    timestamppb.New(d.now()),
		Direction: directionRight,
		SliceSize: uint32(limit), // #nosec G115 -- limit is bounded by the callers
		Filter:    filter,
	})
	if err != nil {
		return nil, fmt.Errorf("can't search runtime events: %w", err)
	}

	return resp.GetRuntimeEvents(), nil
}

// parseTime parses an RFC3339 timestamp, falling back to fallback when empty.
func parseTime(value string, fallback time.Time) (time.Time, error) {
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}

	return parsed, nil
}
