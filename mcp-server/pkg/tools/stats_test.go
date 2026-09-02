package tools

import (
	"context"
	"testing"
	"time"

	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
)

func TestGetRuntimeStatsBreakdown(t *testing.T) {
	t.Parallel()

	stats := &mockRuntimeStats{
		total: 120,
		byType: map[string]int32{
			eventTypeProcessExec:   100,
			eventTypeProcessKprobe: 20,
		},
	}

	deps := newTestDeps(t, nil, stats, nil)

	result, err := getRuntimeStats(deps)(context.Background(), GetRuntimeStatsArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 120 {
		t.Errorf("total = %d, want 120", result.Total)
	}

	// Only the types that actually occurred are reported.
	if len(result.ByType) != 2 {
		t.Fatalf("by_type = %+v, want two entries", result.ByType)
	}
	if result.ByType[0].Type != eventTypeProcessExec || result.ByType[0].Count != 100 {
		t.Errorf("by_type[0] = %+v", result.ByType[0])
	}

	if result.Window.Since != testNow.Add(-defaultLookback).Format(time.RFC3339) {
		t.Errorf("window.since = %q", result.Window.Since)
	}
	if result.Window.Limit != 0 {
		t.Errorf("window.limit = %d, want 0: a counter has no slice size", result.Window.Limit)
	}

	// One call for the total plus one per countable type.
	if want := 1 + len(countableEventTypes); len(stats.reqs) != want {
		t.Errorf("count calls = %d, want %d", len(stats.reqs), want)
	}
	if stats.reqs[0].Type != nil {
		t.Errorf("the first call must ask for the total, got type %q", stats.reqs[0].GetType())
	}
}

func TestGetRuntimeStatsSingleType(t *testing.T) {
	t.Parallel()

	stats := &mockRuntimeStats{total: 120, byType: map[string]int32{eventTypeProcessKprobe: 20}}
	deps := newTestDeps(t, nil, stats, nil)

	result, err := getRuntimeStats(deps)(context.Background(), GetRuntimeStatsArgs{
		EventType: eventTypeProcessKprobe,
		Since:     "2026-08-19T00:00:00Z",
		Until:     "2026-08-20T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 20 || len(result.ByType) != 0 {
		t.Errorf("result = %+v, want only the requested type counted", result)
	}

	if len(stats.reqs) != 1 {
		t.Fatalf("count calls = %d, want 1", len(stats.reqs))
	}

	req := stats.reqs[0]
	if req.GetType() != eventTypeProcessKprobe {
		t.Errorf("type = %q", req.GetType())
	}

	assertPeriod(t, req.GetPeriod(),
		time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
}

func assertPeriod(t *testing.T, period *history_api.Period, from, to time.Time) {
	t.Helper()

	if got := period.GetFrom().AsTime(); !got.Equal(from) {
		t.Errorf("period.from = %v, want %v", got, from)
	}
	if got := period.GetTo().AsTime(); !got.Equal(to) {
		t.Errorf("period.to = %v, want %v", got, to)
	}
}
