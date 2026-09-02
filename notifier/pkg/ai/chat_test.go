package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
)

// The arguments every provider in these tests answers with, and the roles the
// rendered conversations are checked against.
const (
	notificationArguments = `{"query":"notification"}`
	roleUser              = string(RoleUser)
	roleTool              = string(RoleTool)

	// Identifiers and names used across the chat tests.
	firstCallID    = "call_1"
	searchDocsName = "search_docs"

	// Endpoints of the three providers.
	openAIChatPath    = "/chat/completions"
	anthropicChatPath = "/messages"
	ollamaChatPath    = "/api/chat"
)

// searchDocsTool is the tool the chat tests offer the model.
var searchDocsTool = Tool{
	Name:        searchDocsName,
	Description: "Search the product documentation.",
	InputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"query": map[string]any{"type": "string"}},
		"required":   []string{"query"},
	},
}

// chatConversation is a conversation that has already been through one tool
// call, which is the shape every provider has to render correctly.
func chatConversation() []Message {
	return []Message{
		{Role: RoleSystem, Content: "you are an assistant"},
		{Role: RoleUser, Content: "how do I create a notification rule?"},
		{
			Role:      RoleAssistant,
			Content:   "let me look it up",
			ToolCalls: []ToolCall{{ID: firstCallID, Name: searchDocsName, Arguments: json.RawMessage(`{"query":"rule"}`)}},
		},
		{Role: RoleTool, Content: `{"matches":[]}`, ToolCallID: firstCallID, ToolName: searchDocsName},
	}
}

// deltas collects what a streaming answer handed back as it was written.
type deltas struct {
	parts []string
}

func (d *deltas) collect(delta string) {
	d.parts = append(d.parts, delta)
}

func (d *deltas) text() string {
	return strings.Join(d.parts, "")
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request: %v", err)
	}

	return body
}

func TestOpenAICompatibleChat(t *testing.T) {
	t.Parallel()

	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != openAIChatPath {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}

		captured = decodeBody(t, r)

		// The answer arrives as a stream: text in pieces, and a tool call
		// whose name and arguments are split across chunks.
		stream := strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"here "}}]}`,
			`data: {"choices":[{"delta":{"content":"is how"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","function":{"name":"search_docs","arguments":"{\"query\":"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"notification\"}"}}]}}]}`,
			"data: [DONE]",
			"",
		}, "\n\n")

		if _, err := w.Write([]byte(stream)); err != nil {
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

	streamed := &deltas{}

	result, err := client.Chat(context.Background(), chatConversation(), []Tool{searchDocsTool}, streamed.collect)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	// The answer must arrive in pieces, not in one lump at the end.
	if len(streamed.parts) < 2 || streamed.text() != result.Text {
		t.Errorf("streamed %d parts = %q, result text = %q", len(streamed.parts), streamed.text(), result.Text)
	}

	if result.Text != "here is how" {
		t.Errorf("text = %q", result.Text)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}

	call := result.ToolCalls[0]
	if call.ID != "call_2" || call.Name != searchDocsName {
		t.Errorf("call = %+v", call)
	}
	// The provider sends the arguments as a JSON string; the client hands on
	// the object itself, so that a tool never has to unquote them.
	if string(call.Arguments) != notificationArguments {
		t.Errorf("arguments = %s", call.Arguments)
	}

	// The request must carry the tools in the OpenAI function shape.
	tools, ok := captured["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", captured["tools"])
	}
	function, _ := tools[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != searchDocsName || function["parameters"] == nil {
		t.Errorf("function = %#v", function)
	}
	if captured["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %#v", captured["tool_choice"])
	}

	// ... and the conversation, with the previous call and its result.
	messages, _ := captured["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("messages = %d, want the whole conversation", len(messages))
	}

	assistantMessage, _ := messages[2].(map[string]any)
	calls, _ := assistantMessage["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("assistant tool_calls = %#v", assistantMessage["tool_calls"])
	}
	callFunction, _ := calls[0].(map[string]any)["function"].(map[string]any)
	if callFunction["arguments"] != `{"query":"rule"}` {
		t.Errorf("replayed arguments = %#v, want a JSON string", callFunction["arguments"])
	}

	toolMessage, _ := messages[3].(map[string]any)
	if toolMessage["role"] != roleTool || toolMessage["tool_call_id"] != firstCallID {
		t.Errorf("tool message = %#v", toolMessage)
	}
}

func TestAnthropicChat(t *testing.T) {
	t.Parallel()

	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != anthropicChatPath {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}

		captured = decodeBody(t, r)

		stream := strings.Join([]string{
			"event: content_block_start\ndata: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}",
			"event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"look\"}}",
			"event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ing\"}}",
			"event: content_block_start\ndata: {\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"search_docs\"}}",
			"event: content_block_delta\ndata: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\"}}",
			"event: content_block_delta\ndata: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"notification\\\"}\"}}",
			"event: message_stop\ndata: {}",
			"",
		}, "\n\n")

		if _, err := w.Write([]byte(stream)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderAnthropic,
		BaseURL:  server.URL,
		Model:    "claude-sonnet-4",
		APIKey:   "secret",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	streamed := &deltas{}

	result, err := client.Chat(context.Background(), chatConversation(), []Tool{searchDocsTool}, streamed.collect)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	// The answer must arrive in pieces, not in one lump at the end.
	if len(streamed.parts) < 2 || streamed.text() != result.Text {
		t.Errorf("streamed %d parts = %q, result text = %q", len(streamed.parts), streamed.text(), result.Text)
	}

	if result.Text != "looking" {
		t.Errorf("text = %q", result.Text)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "toolu_1" {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
	if string(result.ToolCalls[0].Arguments) != notificationArguments {
		t.Errorf("arguments = %s", result.ToolCalls[0].Arguments)
	}

	// The system prompt is a field of its own, not a message.
	if captured["system"] != "you are an assistant" {
		t.Errorf("system = %#v", captured["system"])
	}

	tools, ok := captured["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", captured["tools"])
	}
	if tools[0].(map[string]any)["input_schema"] == nil {
		t.Errorf("tool has no input_schema: %#v", tools[0])
	}

	// Roles must alternate: user, assistant (with a tool_use block), user
	// (carrying the tool_result block).
	messages, _ := captured["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want the system message dropped and results merged", len(messages))
	}

	roles := make([]any, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.(map[string]any)["role"])
	}
	if roles[0] != roleUser || roles[1] != string(RoleAssistant) || roles[2] != roleUser {
		t.Fatalf("roles = %v", roles)
	}

	assistantBlocks, _ := messages[1].(map[string]any)["content"].([]any)
	if len(assistantBlocks) != 2 {
		t.Fatalf("assistant blocks = %#v", assistantBlocks)
	}
	if assistantBlocks[1].(map[string]any)["type"] != "tool_use" {
		t.Errorf("second block = %#v", assistantBlocks[1])
	}

	resultBlocks, _ := messages[2].(map[string]any)["content"].([]any)
	if len(resultBlocks) != 1 || resultBlocks[0].(map[string]any)["type"] != "tool_result" {
		t.Fatalf("result blocks = %#v", resultBlocks)
	}
	if resultBlocks[0].(map[string]any)["tool_use_id"] != firstCallID {
		t.Errorf("tool_use_id = %#v", resultBlocks[0])
	}
}

// TestAnthropicChatMergesConsecutiveToolResults covers the constraint that
// breaks the Messages API when it is missed: two tools run in one turn produce
// two results, and they have to arrive as one user message.
func TestAnthropicChatMergesConsecutiveToolResults(t *testing.T) {
	t.Parallel()

	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = decodeBody(t, r)

		stream := "event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"done\"}}\n\n"

		if _, err := w.Write([]byte(stream)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderAnthropic,
		BaseURL:  server.URL,
		Model:    "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	messages := []Message{
		{Role: RoleUser, Content: "what happened?"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "a", Name: "search_runtime_events", Arguments: json.RawMessage(`{}`)},
			{ID: "b", Name: "list_detectors", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: RoleTool, Content: "events", ToolCallID: "a", ToolName: "search_runtime_events"},
		{Role: RoleTool, Content: "detectors", ToolCallID: "b", ToolName: "list_detectors"},
	}

	if _, err := client.Chat(context.Background(), messages, nil, nil); err != nil {
		t.Fatalf("chat: %v", err)
	}

	rendered, _ := captured["messages"].([]any)
	if len(rendered) != 3 {
		t.Fatalf("messages = %d, want both results merged into one user turn", len(rendered))
	}

	blocks, _ := rendered[2].(map[string]any)["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("result blocks = %#v", blocks)
	}
}

func TestOllamaChat(t *testing.T) {
	t.Parallel()

	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ollamaChatPath {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}

		captured = decodeBody(t, r)

		// Ollama streams newline-delimited JSON, sends the arguments as an
		// object and gives no call identifier.
		stream := strings.Join([]string{
			`{"message":{"content":"look"},"done":false}`,
			`{"message":{"content":"ing"},"done":false}`,
			`{"message":{"content":"","tool_calls":[{"function":{"name":"search_docs","arguments":{"query":"notification"}}}]},"done":false}`,
			`{"done":true}`,
			"",
		}, "\n")

		if _, err := w.Write([]byte(stream)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(&model.AI{
		Provider: model.AIProviderOllama,
		BaseURL:  server.URL,
		Model:    "qwen3",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	streamed := &deltas{}

	result, err := client.Chat(context.Background(), chatConversation(), []Tool{searchDocsTool}, streamed.collect)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	// The answer must arrive in pieces, not in one lump at the end.
	if len(streamed.parts) < 2 || streamed.text() != result.Text {
		t.Errorf("streamed %d parts = %q, result text = %q", len(streamed.parts), streamed.text(), result.Text)
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
	if result.ToolCalls[0].ID == "" {
		t.Error("call has no id: the loop needs one to correlate the result")
	}
	if string(result.ToolCalls[0].Arguments) != notificationArguments {
		t.Errorf("arguments = %s", result.ToolCalls[0].Arguments)
	}

	messages, _ := captured["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("messages = %d", len(messages))
	}

	assistantMessage, _ := messages[2].(map[string]any)
	calls, _ := assistantMessage["tool_calls"].([]any)
	callFunction, _ := calls[0].(map[string]any)["function"].(map[string]any)
	// Ollama takes the arguments as an object, unlike OpenAI.
	if _, ok := callFunction["arguments"].(map[string]any); !ok {
		t.Errorf("replayed arguments = %#v, want an object", callFunction["arguments"])
	}

	toolMessage, _ := messages[3].(map[string]any)
	if toolMessage["tool_name"] != searchDocsName {
		t.Errorf("tool message = %#v, want it correlated by name", toolMessage)
	}
}

func TestUnquoteArguments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "object", raw: `{"a":1}`, want: `{"a":1}`},
		{name: "json string", raw: `"{\"a\":1}"`, want: `{"a":1}`},
		{name: "empty", raw: ``, want: `{}`},
		{name: "null", raw: `null`, want: `{}`},
		{name: "empty string", raw: `""`, want: `{}`},
		{name: "not an object", raw: `[1,2]`, want: `{}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := string(unquoteArguments(json.RawMessage(tc.raw))); got != tc.want {
				t.Errorf("arguments = %s, want %s", got, tc.want)
			}
		})
	}
}
