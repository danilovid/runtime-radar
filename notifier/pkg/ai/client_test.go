package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
)

func TestOpenAICompatibleExplainRuntimeEvent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %s", r.Header.Get("Authorization"))
		}
		_, err := w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"s\",\"risk\":\"medium\",\"possible_cause\":\"c\",\"next_steps\":[\"a\",\"b\"]}"}}]}`))
		if err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOpenAICompatible,
		BaseURL:  server.URL,
		Model:    "gpt-4.1",
		APIKey:   "secret",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ExplainRuntimeEvent(context.Background(), "event-1", `{"id":"event-1"}`)
	if err != nil {
		t.Fatalf("explain runtime event: %v", err)
	}
	if result.Summary != "s" {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
	if result.Risk != "medium" {
		t.Fatalf("unexpected risk: %s", result.Risk)
	}
	if result.PossibleCause != "c" {
		t.Fatalf("unexpected possible cause: %s", result.PossibleCause)
	}
	if !reflect.DeepEqual(result.NextSteps, []string{"a", "b"}) {
		t.Fatalf("unexpected next steps: %#v", result.NextSteps)
	}
}

func TestAnthropicExplainRuntimeEvent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "secret" {
			t.Fatalf("unexpected api key header: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Fatalf("unexpected anthropic version: %s", r.Header.Get("anthropic-version"))
		}
		_, err := w.Write([]byte(`{"content":[{"type":"text","text":"{\"summary\":\"s\",\"risk\":\"low\",\"possible_cause\":\"c\",\"next_steps\":[\"a\"]}"}]}`))
		if err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderAnthropic,
		BaseURL:  server.URL,
		Model:    "claude-3-7-sonnet",
		APIKey:   "secret",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ExplainRuntimeEvent(context.Background(), "event-1", `{"id":"event-1"}`)
	if err != nil {
		t.Fatalf("explain runtime event: %v", err)
	}
	if result.Summary != "s" {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
	if result.Risk != "low" {
		t.Fatalf("unexpected risk: %s", result.Risk)
	}
	if result.PossibleCause != "c" {
		t.Fatalf("unexpected possible cause: %s", result.PossibleCause)
	}
	if !reflect.DeepEqual(result.NextSteps, []string{"a"}) {
		t.Fatalf("unexpected next steps: %#v", result.NextSteps)
	}
}

func TestOllamaExplainRuntimeEvent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, err := w.Write([]byte(`{"message":{"content":"{\"summary\":\"s\",\"risk\":\"high\",\"possible_cause\":\"c\",\"next_steps\":[\"a\"]}"}}`))
		if err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOllama,
		BaseURL:  server.URL,
		Model:    "llama3.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ExplainRuntimeEvent(context.Background(), "event-1", `{"id":"event-1"}`)
	if err != nil {
		t.Fatalf("explain runtime event: %v", err)
	}
	if result.Summary != "s" {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
	if result.Risk != "high" {
		t.Fatalf("unexpected risk: %s", result.Risk)
	}
	if result.PossibleCause != "c" {
		t.Fatalf("unexpected possible cause: %s", result.PossibleCause)
	}
	if !reflect.DeepEqual(result.NextSteps, []string{"a"}) {
		t.Fatalf("unexpected next steps: %#v", result.NextSteps)
	}
}
