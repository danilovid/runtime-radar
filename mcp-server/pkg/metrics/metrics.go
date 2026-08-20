// Package metrics holds the Prometheus collectors of the service. Every MCP
// tool call is counted, timed and, when it fails, counted again by gRPC status
// code, so that a misbehaving agent is visible without reading the logs.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/status"
)

const namespace = "mcp_server"

var (
	toolCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tool_calls_total",
		Help:      "Total number of MCP tool calls.",
	}, []string{"tool"})

	toolErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tool_errors_total",
		Help:      "Total number of failed MCP tool calls labeled with a gRPC status code.",
	}, []string{"tool", "code"})

	toolDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "tool_duration_seconds",
		Help:      "Duration of MCP tool calls.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"tool"})
)

// Collectors returns the collectors of the service to be registered in the
// metrics registry.
func Collectors() []prometheus.Collector {
	return []prometheus.Collector{toolCalls, toolErrors, toolDuration}
}

// ObserveToolCall records a single tool call. A nil err means the call
// succeeded, otherwise the error is also counted by its gRPC status code.
func ObserveToolCall(tool string, d time.Duration, err error) {
	toolCalls.WithLabelValues(tool).Inc()
	toolDuration.WithLabelValues(tool).Observe(d.Seconds())

	if err != nil {
		toolErrors.WithLabelValues(tool, status.Code(err).String()).Inc()
	}
}
