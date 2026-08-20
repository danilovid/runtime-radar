// Package metrics holds the Prometheus collectors of the service.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "notifier"

var (
	assistantChats = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "assistant",
		Name:      "chats_total",
		Help:      "Total number of assistant chats, by how they ended.",
	}, []string{"stop_reason"})

	assistantErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "assistant",
		Name:      "errors_total",
		Help:      "Total number of failed assistant chats, by what failed.",
	}, []string{"reason"})

	assistantIterations = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "assistant",
		Name:      "iterations",
		Help:      "Number of model turns one assistant chat took.",
		Buckets:   []float64{1, 2, 3, 4, 5, 6, 7, 8},
	})

	assistantDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "assistant",
		Name:      "duration_seconds",
		Help:      "Duration of an assistant chat.",
		Buckets:   []float64{1, 2.5, 5, 10, 20, 30, 60, 120},
	})

	assistantActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "assistant",
		Name:      "active_chats",
		Help:      "Number of assistant chats running right now.",
	})

	assistantToolCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "assistant",
		Name:      "tool_calls_total",
		Help:      "Total number of MCP tool calls made by the assistant.",
	}, []string{"tool", "status"})

	assistantToolDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "assistant",
		Name:      "tool_duration_seconds",
		Help:      "Duration of an MCP tool call made by the assistant.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"tool"})
)

// Collectors returns the collectors of the service to be registered in the
// metrics registry.
func Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		assistantChats,
		assistantErrors,
		assistantIterations,
		assistantDuration,
		assistantActive,
		assistantToolCalls,
		assistantToolDuration,
	}
}

// ObserveAssistantChat records a finished chat.
func ObserveAssistantChat(stopReason string, iterations int, d time.Duration) {
	assistantChats.WithLabelValues(stopReason).Inc()
	assistantIterations.Observe(float64(iterations))
	assistantDuration.Observe(d.Seconds())
}

// ObserveAssistantError records a chat that could not be answered. The reason
// is a short constant, never an error message: those carry user data.
func ObserveAssistantError(reason string) {
	assistantErrors.WithLabelValues(reason).Inc()
}

// AssistantChatStarted and AssistantChatFinished track how many chats are in
// flight, which is what the concurrency limit is set against.
func AssistantChatStarted() {
	assistantActive.Inc()
}

func AssistantChatFinished() {
	assistantActive.Dec()
}

// ObserveAssistantToolCall records one tool call.
func ObserveAssistantToolCall(tool string, d time.Duration, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}

	assistantToolCalls.WithLabelValues(tool, status).Inc()
	assistantToolDuration.WithLabelValues(tool).Observe(d.Seconds())
}
