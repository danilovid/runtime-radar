package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// countableEventTypes are the event types Runtime Stats can count. UNDEF is
// left out: History API rejects it as a counter type.
var countableEventTypes = []string{
	eventTypeProcessExec,
	eventTypeProcessExit,
	eventTypeProcessKprobe,
	eventTypeProcessTracepoint,
	eventTypeProcessLoader,
	eventTypeProcessUprobe,
}

// GetRuntimeStatsArgs are the arguments of get_runtime_stats.
type GetRuntimeStatsArgs struct {
	Since     string `json:"since,omitempty" jsonschema:"start of the time window, RFC3339. Defaults to 24 hours before now"`
	Until     string `json:"until,omitempty" jsonschema:"end of the time window, RFC3339. Defaults to now"`
	EventType string `json:"event_type,omitempty" jsonschema:"count only events of this type: PROCESS_EXEC, PROCESS_EXIT, PROCESS_KPROBE, PROCESS_TRACEPOINT, PROCESS_LOADER or PROCESS_UPROBE. Omit to also get a breakdown by type"`
}

// TypeCount is the number of events of one type in the window.
type TypeCount struct {
	Type  string `json:"type" jsonschema:"type of the runtime event"`
	Count int32  `json:"count" jsonschema:"number of events of that type in the window"`
}

// GetRuntimeStatsResult is the answer of get_runtime_stats.
type GetRuntimeStatsResult struct {
	Total  int32        `json:"total" jsonschema:"number of runtime events recorded in the window"`
	ByType []TypeCount  `json:"by_type,omitempty" jsonschema:"breakdown by event type, returned when no event_type was given"`
	Window SearchWindow `json:"window" jsonschema:"the time window the counts cover"`
}

func registerStatsTools(server *mcp.Server, deps *Deps) {
	addTool(server, deps, &mcp.Tool{
		Name:        "get_runtime_stats",
		Annotations: readOnly("Get runtime event stats"),
		Description: "Count the runtime events recorded in a time window, in total and broken down by event type. " +
			"Use it to size a problem before searching: how much activity a window holds, and whether the volume " +
			"is worth narrowing down before calling search_runtime_events.",
	}, []auth.Permission{auth.ReadEvents()}, getRuntimeStats(deps))
}

func getRuntimeStats(deps *Deps) func(context.Context, GetRuntimeStatsArgs) (GetRuntimeStatsResult, error) {
	return func(ctx context.Context, args GetRuntimeStatsArgs) (GetRuntimeStatsResult, error) {
		window, period, err := args.period(deps.now())
		if err != nil {
			return GetRuntimeStatsResult{}, err
		}

		result := GetRuntimeStatsResult{Window: window}

		if args.EventType != "" {
			eventType := args.EventType

			counter, err := deps.Clients.RuntimeStats.CountEvents(ctx, &history_api.RuntimeEventsCounterReq{
				Period: period,
				Type:   &eventType,
			})
			if err != nil {
				return GetRuntimeStatsResult{}, fmt.Errorf("can't count runtime events: %w", err)
			}

			result.Total = counter.GetCount()

			return result, nil
		}

		counter, err := deps.Clients.RuntimeStats.CountEvents(ctx, &history_api.RuntimeEventsCounterReq{Period: period})
		if err != nil {
			return GetRuntimeStatsResult{}, fmt.Errorf("can't count runtime events: %w", err)
		}

		result.Total = counter.GetCount()

		for _, eventType := range countableEventTypes {
			byType, err := deps.Clients.RuntimeStats.CountEvents(ctx, &history_api.RuntimeEventsCounterReq{
				Period: period,
				Type:   &eventType,
			})
			if err != nil {
				return GetRuntimeStatsResult{}, fmt.Errorf("can't count %s events: %w", eventType, err)
			}

			if byType.GetCount() == 0 {
				continue
			}

			result.ByType = append(result.ByType, TypeCount{Type: eventType, Count: byType.GetCount()})
		}

		return result, nil
	}
}

// period resolves the time window of the request the same way a search does.
func (a GetRuntimeStatsArgs) period(now time.Time) (SearchWindow, *history_api.Period, error) {
	search := SearchRuntimeEventsArgs{Since: a.Since, Until: a.Until}

	req, window, err := search.request(now)
	if err != nil {
		return SearchWindow{}, nil, err
	}

	// The counter API has no slice size; the limit of the search window is
	// meaningless here.
	window.Limit = 0

	period := req.GetFilter().GetPeriod()
	if period == nil {
		period = &history_api.Period{From: timestamppb.New(now.Add(-defaultLookback)), To: timestamppb.New(now)}
	}

	return window, period, nil
}
