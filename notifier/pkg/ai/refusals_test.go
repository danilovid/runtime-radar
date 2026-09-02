package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
)

func newOpenAIClientFor(baseURL string) *openAICompatibleClient {
	return &openAICompatibleClient{baseClient: &baseClient{
		conf:    &model.AI{Model: "test"},
		baseURL: baseURL,
	}}
}

func TestMaxTokensFieldFollowsEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1":                         maxCompletionTokensField,
		"https://API.OpenAI.com/v1":                         maxCompletionTokensField,
		"https://api.deepseek.com/v1":                       maxTokensFieldName,
		"http://localhost:8000/v1":                          maxTokensFieldName,
		"https://dashscope.aliyuncs.com/compatible-mode/v1": maxTokensFieldName,
	}

	for baseURL, want := range cases {
		if got := newOpenAIClientFor(baseURL).maxTokensField(); got != want {
			t.Errorf("maxTokensField() for %q = %q, want %q", baseURL, got, want)
		}
	}
}

func TestIsUnsupportedMaxTokens(t *testing.T) {
	openAI := fmt.Errorf("unexpected status 400: {\n  \"error\": {\n    \"message\": " +
		"\"Unsupported parameter: 'max_tokens' is not supported with this model. " +
		"Use 'max_completion_tokens' instead.\",\n    \"type\": \"invalid_request_error\",\n    " +
		"\"param\": \"max_tokens\",\n    \"code\": \"unsupported_parameter\"\n  }\n}")

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "openai wording", err: openAI, want: true},
		{name: "gateway echoes the parameter only", err: fmt.Errorf(
			`unexpected status 400: {"error":{"param":"max_tokens","code":"unsupported_parameter"}}`), want: true},
		{name: "unrelated failure", err: fmt.Errorf("unexpected status 401: invalid api key"), want: false},
		{name: "another parameter refused", err: fmt.Errorf(
			`unexpected status 400: {"error":{"param":"temperature","code":"unsupported_value"}}`), want: false},
	}

	for _, tc := range cases {
		if got := isUnsupportedMaxTokens(tc.err); got != tc.want {
			t.Errorf("%s: isUnsupportedMaxTokens() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A gateway is not recognised by its host, so the refusal is the only signal,
// and it must be acted on exactly once.
func TestRetryWithCompletionTokensSwitchesOnce(t *testing.T) {
	c := newOpenAIClientFor("https://llm.example.internal/v1")
	if got := c.maxTokensField(); got != maxTokensFieldName {
		t.Fatalf("before the refusal maxTokensField() = %q, want %q", got, maxTokensFieldName)
	}

	refusal := fmt.Errorf("unexpected status 400: Use 'max_completion_tokens' instead of 'max_tokens'")
	if !c.retryAfter(refusal) {
		t.Fatal("the first refusal must be retried")
	}

	if got := c.maxTokensField(); got != maxCompletionTokensField {
		t.Errorf("after the refusal maxTokensField() = %q, want %q", got, maxCompletionTokensField)
	}

	if c.retryAfter(refusal) {
		t.Error("the same refusal must not be retried twice")
	}

	if c.retryAfter(nil) {
		t.Error("a request that succeeded must not be retried")
	}
}

// The point of the fallback is that a gateway which refuses max_tokens is
// answered by repeating the request, not by failing the call.
func TestCompleteRetriesOnRefusedMaxTokens(t *testing.T) {
	t.Parallel()

	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)

			return
		}

		switch {
		case body[maxCompletionTokensField] != nil:
			seen = append(seen, maxCompletionTokensField)

			if _, err := w.Write([]byte(`{"choices":[{"message":{"content":` +
				`"{\"summary\":\"s\",\"risk\":\"medium\",\"possible_cause\":\"c\",\"next_steps\":[\"a\"]}"}}]}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case body[maxTokensFieldName] != nil:
			seen = append(seen, maxTokensFieldName)

			w.WriteHeader(http.StatusBadRequest)

			if _, err := w.Write([]byte(`{"error":{"message":"Unsupported parameter: 'max_tokens' is not ` +
				`supported with this model. Use 'max_completion_tokens' instead.","param":"max_tokens",` +
				`"code":"unsupported_parameter"}}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Error("request carried no output budget at all")
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOpenAICompatible,
		BaseURL:  server.URL,
		Model:    "gpt-5",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if _, err := client.ExplainRuntimeEvent(context.Background(), "id", `{"a":1}`); err != nil {
		t.Fatalf("explain runtime event: %v", err)
	}

	want := []string{maxTokensFieldName, maxCompletionTokensField}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("budget names sent: %v, want %v", seen, want)
	}

	// The client remembers, so a second call goes straight to the new name.
	if _, err := client.ExplainRuntimeEvent(context.Background(), "id", `{"a":1}`); err != nil {
		t.Fatalf("second explain runtime event: %v", err)
	}

	want = append(want, maxCompletionTokensField)
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("budget names sent: %v, want %v", seen, want)
	}
}

func TestIsUnsupportedTemperature(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "openai wording", err: fmt.Errorf(`unexpected status 400: {"error":{"message":` +
			`"Unsupported value: 'temperature' does not support 0.1 with this model. Only the default (1) ` +
			`value is supported.","param":"temperature","code":"unsupported_value"}}`), want: true},
		{name: "unrelated value refused", err: fmt.Errorf(
			`unexpected status 400: {"error":{"param":"top_p","code":"unsupported_value"}}`), want: false},
		{name: "budget refused", err: fmt.Errorf(
			`unexpected status 400: {"error":{"param":"max_tokens","code":"unsupported_parameter"}}`), want: false},
	}

	for _, tc := range cases {
		if got := isUnsupportedTemperature(tc.err); got != tc.want {
			t.Errorf("%s: isUnsupportedTemperature() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// An endpoint reports one refused argument at a time, so a model that takes
// neither of them has to be answered twice before the call can go through.
func TestCompleteRetriesUntilTheRequestIsAccepted(t *testing.T) {
	t.Parallel()

	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)

			return
		}

		attempts++

		switch {
		case body[maxTokensFieldName] != nil:
			w.WriteHeader(http.StatusBadRequest)
			writeOrFail(t, w, `{"error":{"param":"max_tokens","code":"unsupported_parameter"}}`)
		case body["temperature"] != nil:
			w.WriteHeader(http.StatusBadRequest)
			writeOrFail(t, w, `{"error":{"message":"Unsupported value: 'temperature' does not support 0.1 `+
				`with this model.","param":"temperature","code":"unsupported_value"}}`)
		default:
			writeOrFail(t, w, `{"choices":[{"message":{"content":`+
				`"{\"summary\":\"s\",\"risk\":\"medium\",\"possible_cause\":\"c\",\"next_steps\":[\"a\"]}"}}]}`)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOpenAICompatible,
		BaseURL:  server.URL,
		Model:    "reasoning",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if _, err := client.ExplainRuntimeEvent(context.Background(), "id", `{"a":1}`); err != nil {
		t.Fatalf("explain runtime event: %v", err)
	}

	// max_tokens refused, then temperature refused, then accepted.
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func writeOrFail(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func TestIsToolsNeedNoReasoning(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "openai wording", err: fmt.Errorf(`unexpected status 400: {"error":{"message":"Function tools ` +
			`with reasoning_effort are not supported for gpt-5.6-luna in /v1/chat/completions. To use function ` +
			`tools, use /v1/responses or set reasoning_effort to 'none'.","param":"reasoning_effort"}}`), want: true},
		{name: "budget refused", err: fmt.Errorf(
			`unexpected status 400: {"error":{"param":"max_tokens","code":"unsupported_parameter"}}`), want: false},
		{name: "temperature refused", err: fmt.Errorf(
			`unexpected status 400: {"error":{"param":"temperature","code":"unsupported_value"}}`), want: false},
	}

	for _, tc := range cases {
		if got := isToolsNeedNoReasoning(tc.err); got != tc.want {
			t.Errorf("%s: isToolsNeedNoReasoning() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Reasoning is turned off only for a request that carries tools, and only once
// the endpoint has said it wants that.
func TestSetToolReasoning(t *testing.T) {
	c := newOpenAIClientFor("https://api.openai.com/v1")

	reqBody := map[string]any{}
	c.setToolReasoning(reqBody)

	if _, ok := reqBody[reasoningEffortField]; ok {
		t.Error("reasoning was turned off before the endpoint asked for it")
	}

	refusal := fmt.Errorf("unexpected status 400: Function tools with reasoning_effort are not supported")
	if !c.retryAfter(refusal) {
		t.Fatal("the refusal must be retried")
	}

	c.setToolReasoning(reqBody)

	if reqBody[reasoningEffortField] != reasoningEffortNone {
		t.Errorf("reasoning_effort = %v, want %q", reqBody[reasoningEffortField], reasoningEffortNone)
	}

	if c.retryAfter(refusal) {
		t.Error("the same refusal must not be retried twice")
	}
}
