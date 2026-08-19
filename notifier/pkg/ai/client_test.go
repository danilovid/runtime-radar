package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
)

func TestOpenAICompatibleExplainRuntimeEvent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected authorization header: %s", r.Header.Get("Authorization"))
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		kwargs, _ := body["chat_template_kwargs"].(map[string]any)
		if kwargs["enable_thinking"] != false {
			t.Errorf("expected enable_thinking=false, got %#v", body["chat_template_kwargs"])
			return
		}

		if _, err := w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"s\",\"risk\":\"medium\",\"possible_cause\":\"c\",\"next_steps\":[\"a\",\"b\"]}"}}]}`)); err != nil {
			t.Errorf("write response: %v", err)
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

func TestOpenAICompatibleFallsBackToReasoningContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{"choices":[{"message":{"content":"","reasoning_content":"{\"summary\":\"s\",\"risk\":\"low\",\"possible_cause\":\"c\",\"next_steps\":[\"a\"]}"}}]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOpenAICompatible,
		BaseURL:  server.URL,
		Model:    "positive-llm-experimental",
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
}

func TestAnthropicExplainRuntimeEvent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		if r.Header.Get("x-api-key") != "secret" {
			t.Errorf("unexpected api key header: %s", r.Header.Get("x-api-key"))
			return
		}
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("unexpected anthropic version: %s", r.Header.Get("anthropic-version"))
			return
		}
		if _, err := w.Write([]byte(`{"content":[{"type":"text","text":"{\"summary\":\"s\",\"risk\":\"low\",\"possible_cause\":\"c\",\"next_steps\":[\"a\"]}"}]}`)); err != nil {
			t.Errorf("write response: %v", err)
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
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		if _, err := w.Write([]byte(`{"message":{"content":"{\"summary\":\"s\",\"risk\":\"high\",\"possible_cause\":\"c\",\"next_steps\":[\"a\"]}"}}`)); err != nil {
			t.Errorf("write response: %v", err)
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

func TestIsOfficialOpenAI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{"default base url", defaultOpenAIBaseURL, true},
		{"explicit openai host", "https://api.openai.com/v1", true},
		{"mixed case host", "https://API.OpenAI.com/v1", true},
		{"vllm host", "http://vllm.ai-ns.svc:8000/v1", false},
		{"azure openai", "https://contoso.openai.azure.com/openai", false},
		{"look-alike host", "https://api.openai.com.evil.test/v1", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isOfficialOpenAI(tc.baseURL); got != tc.want {
				t.Fatalf("isOfficialOpenAI(%q) = %v, want %v", tc.baseURL, got, tc.want)
			}
		})
	}
}

func TestIsLocalEndpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		provider model.AIProvider
		baseURL  string
		want     bool
	}{
		{"loopback ip", model.AIProviderOpenAICompatible, "http://127.0.0.1:8000/v1", true},
		{"ipv6 loopback", model.AIProviderOpenAICompatible, "http://[::1]:8000/v1", true},
		{"private ip", model.AIProviderOpenAICompatible, "http://10.0.0.5:8000/v1", true},
		{"link local ip", model.AIProviderOpenAICompatible, "http://169.254.169.254/v1", true},
		{"localhost", model.AIProviderOpenAICompatible, "http://localhost:11434", true},
		{"single label service", model.AIProviderOpenAICompatible, "http://ollama:11434", true},
		{"cluster service", model.AIProviderOpenAICompatible, "http://vllm.ai-ns.svc:8000/v1", true},
		{"cluster fqdn", model.AIProviderOpenAICompatible, "http://vllm.ai-ns.svc.cluster.local:8000/v1", true},
		{"trailing dot", model.AIProviderOpenAICompatible, "http://vllm.ai-ns.svc.", true},
		{"public ip", model.AIProviderOpenAICompatible, "http://8.8.8.8:8000/v1", false},
		{"public name", model.AIProviderOpenAICompatible, "https://api.openai.com/v1", false},
		{"internal-looking public name", model.AIProviderOpenAICompatible, "https://ollama.corp.example.com", false},
		{"ollama default", model.AIProviderOllama, "", true},
		{"anthropic default", model.AIProviderAnthropic, "", false},
		{"openai default", model.AIProviderOpenAICompatible, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conf := &model.AI{Provider: tc.provider, BaseURL: tc.baseURL}
			if got := IsLocalEndpoint(conf); got != tc.want {
				t.Fatalf("IsLocalEndpoint(%s, %q) = %v, want %v", tc.provider, tc.baseURL, got, tc.want)
			}
		})
	}
}

func TestNewClientRejectsNonLocalEndpointMarkedLocal(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(&model.AI{
		Provider: model.AIProviderOpenAICompatible,
		BaseURL:  "https://api.openai.com/v1",
		Model:    "gpt-4.1",
		IsLocal:  true,
	}); err == nil {
		t.Fatal("expected an error for a public endpoint marked local")
	}

	if _, err := NewClient(&model.AI{
		Provider: model.AIProviderOpenAICompatible,
		BaseURL:  "http://vllm.ai-ns.svc:8000/v1",
		Model:    "qwen3",
		IsLocal:  true,
	}); err != nil {
		t.Fatalf("unexpected error for an in-cluster endpoint marked local: %v", err)
	}
}

func TestParseResultReportsUnparsedOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		raw         string
		wantParsed  bool
		wantSummary string
	}{
		{"plain json", `{"summary":"s","risk":"low"}`, true, "s"},
		{"fenced json", "```json\n{\"summary\":\"s\",\"risk\":\"low\"}\n```", true, "s"},
		{"prose preamble", "Sure, here you go: {\"summary\":\"s\"}", false, ""},
		{"chain of thought", "Okay, the user wants me to analyse this event...", false, ""},
		{"empty", "", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, parsed := parseResult(tc.raw)
			if parsed != tc.wantParsed {
				t.Fatalf("parsed = %v, want %v", parsed, tc.wantParsed)
			}
			// Unparsed output must never masquerade as an analyst summary,
			// but it still has to reach the caller as raw text.
			if result.Summary != tc.wantSummary {
				t.Fatalf("summary = %q, want %q", result.Summary, tc.wantSummary)
			}
			if result.RawText != tc.raw {
				t.Fatalf("raw text = %q, want %q", result.RawText, tc.raw)
			}
		})
	}
}

func TestTestFailsOnUnparsableResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{"choices":[{"message":{"content":"Sure! I am a helpful assistant."}}]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOpenAICompatible,
		BaseURL:  server.URL,
		Model:    "chatty",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.Test(context.Background()); err == nil {
		t.Fatal("expected Test to fail on a reachable endpoint that can't produce the JSON")
	}
}

func TestAnthropicFailsWhenNoTextBlocks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{"content":[{"type":"thinking","text":""}]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderAnthropic,
		BaseURL:  server.URL,
		Model:    "claude-3-7-sonnet",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.Test(context.Background()); err == nil {
		t.Fatal("expected an error when the response carries no text blocks")
	}

	if _, err := client.ExplainRuntimeEvent(context.Background(), "event-1", `{"id":"event-1"}`); err == nil {
		t.Fatal("expected an error when the response carries no text blocks")
	}
}

func TestOllamaSendsAPIKey(t *testing.T) {
	t.Parallel()

	authorization := make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")

		if _, err := w.Write([]byte(`{"message":{"content":"{\"summary\":\"s\"}"}}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOllama,
		BaseURL:  server.URL,
		Model:    "llama3.1",
		APIKey:   "secret",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if _, err := client.ExplainRuntimeEvent(context.Background(), "event-1", `{"id":"event-1"}`); err != nil {
		t.Fatalf("explain runtime event: %v", err)
	}

	if got := <-authorization; got != "Bearer secret" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func TestTrimEventJSONCutsOnRuneBoundary(t *testing.T) {
	t.Parallel()

	if short := `{"id":"событие"}`; trimEventJSON(short) != short {
		t.Fatalf("a document under the limit must be passed through unchanged")
	}

	// Fill so that the byte at the cut lands in the middle of a 2-byte rune.
	head := strings.Repeat("a", maxEventJSONBytes-1)
	trimmed := trimEventJSON(head + "яяя")

	if !utf8.ValidString(trimmed) {
		t.Fatal("trimmed event json is not valid utf-8")
	}
	if !strings.HasSuffix(trimmed, truncationMarker) {
		t.Fatalf("trimmed event json does not say it was truncated: %q", trimmed[len(trimmed)-20:])
	}
	if body := strings.TrimSuffix(trimmed, truncationMarker); body != head {
		t.Fatalf("expected the cut to back off to the rune boundary, got %d bytes", len(body))
	}
}
