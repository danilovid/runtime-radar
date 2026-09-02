package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// chatMaxTokens is the answer budget of one assistant turn. It's larger
	// than an event explanation gets, because a chat answer may have to
	// summarise several tool results at once.
	chatMaxTokens = 2048

	// chatTemperature is deliberately low: the assistant reports what the tools
	// returned, it does not brainstorm.
	chatTemperature = 0.2
)

// Roles of a chat conversation. They are the OpenAI names; the Anthropic
// client maps them onto its own shape.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Anthropic content block types used on both sides of the Messages API.
const (
	anthropicBlockText       = "text"
	anthropicBlockToolUse    = "tool_use"
	anthropicBlockToolResult = "tool_result"

	// toolChoiceAuto lets the model decide whether to use a tool at all.
	toolChoiceAuto = "auto"
)

// Role is who produced a chat message.
type Role string

// ToolCall is the model asking for a tool to be run.
type ToolCall struct {
	// ID correlates the call with its result. Providers that don't supply one
	// (Ollama) get an ID synthesised by the client, so that the rest of the
	// code has one shape to work with.
	ID string
	// Name is the tool to run.
	Name string
	// Arguments is the JSON object the tool is called with. It comes from the
	// model, so it is neither trusted nor assumed to match the tool's schema.
	Arguments json.RawMessage
}

// Tool is a tool offered to the model. Name, Description and InputSchema are
// what the provider is sent; the rest describes what running the tool would do
// and never leaves this service — it is what the agent loop decides on before
// making the call.
type Tool struct {
	Name        string
	Description string
	// InputSchema is a JSON Schema object describing the tool's arguments.
	InputSchema map[string]any
	// Title is the tool's human readable name, shown to the user when the loop
	// asks whether to run it.
	Title string
	// ReadOnly is set when the tool only looks things up. A tool that is not
	// read-only changes the product and is never run without the user's
	// explicit approval.
	ReadOnly bool
	// Destructive is set when the tool removes something rather than adds it.
	Destructive bool
}

// Message is one turn of a conversation.
type Message struct {
	Role    Role
	Content string
	// ToolCalls is set on an assistant turn that asked for tools.
	ToolCalls []ToolCall
	// ToolCallID and ToolName identify which call a RoleTool message answers.
	ToolCallID string
	ToolName   string
}

// DeltaFunc receives the answer as it is written. It may be nil, in which case
// the answer is only returned whole.
type DeltaFunc func(delta string)

// emitDelta hands a piece of the answer to the caller, if there is one.
func emitDelta(onDelta DeltaFunc, delta string) {
	if onDelta != nil {
		onDelta(delta)
	}
}

// ChatResult is one answer from the model: either text, or a request to run
// tools, or both (a model may narrate what it is about to do).
type ChatResult struct {
	Text      string
	ToolCalls []ToolCall
}

// toolCallID synthesises an identifier for providers that don't return one.
func toolCallID(index int, name string) string {
	return fmt.Sprintf("call_%d_%s", index, name)
}

// systemPrompt joins every system message of a conversation. Providers that
// take the system prompt as a separate field read it from here.
func systemPrompt(messages []Message) string {
	parts := make([]string, 0, 1)
	for _, message := range messages {
		if message.Role == RoleSystem && message.Content != "" {
			parts = append(parts, message.Content)
		}
	}

	return strings.Join(parts, "\n\n")
}

// openAITools renders tools in the shape OpenAI-compatible backends and Ollama
// both accept.
func openAITools(tools []Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}

	rendered := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		rendered = append(rendered, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema,
			},
		})
	}

	return rendered
}

// openAIMessages renders a conversation in the OpenAI chat shape. Ollama takes
// the same shape, with the two differences openAIStyle covers.
func openAIMessages(messages []Message, ollama bool) []map[string]any {
	rendered := make([]map[string]any, 0, len(messages))

	for _, message := range messages {
		item := map[string]any{"role": string(message.Role)}

		switch message.Role {
		case RoleTool:
			item["content"] = message.Content
			if ollama {
				// Ollama correlates a result with a call by tool name, having
				// no call identifiers of its own.
				item["tool_name"] = message.ToolName
			} else {
				item["tool_call_id"] = message.ToolCallID
			}

		case RoleAssistant:
			item["content"] = message.Content

			if len(message.ToolCalls) > 0 {
				calls := make([]map[string]any, 0, len(message.ToolCalls))
				for _, call := range message.ToolCalls {
					function := map[string]any{"name": call.Name}

					if ollama {
						// Ollama takes the arguments as an object, OpenAI as a
						// string holding JSON.
						function["arguments"] = json.RawMessage(call.Arguments)
					} else {
						function["arguments"] = string(call.Arguments)
					}

					calls = append(calls, map[string]any{
						"id":       call.ID,
						"type":     "function",
						"function": function,
					})
				}

				item["tool_calls"] = calls
			}

		default:
			item["content"] = message.Content
		}

		rendered = append(rendered, item)
	}

	return rendered
}

// openAIToolCalls reads the tool calls out of an OpenAI-style answer.
type openAIToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name string `json:"name"`
		// OpenAI sends a JSON string, Ollama a JSON object; RawMessage takes
		// either and unquoteArguments normalises it.
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

func parseOpenAIToolCalls(calls []openAIToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}

	parsed := make([]ToolCall, 0, len(calls))
	for i, call := range calls {
		if call.Function.Name == "" {
			continue
		}

		id := call.ID
		if id == "" {
			id = toolCallID(i, call.Function.Name)
		}

		parsed = append(parsed, ToolCall{
			ID:        id,
			Name:      call.Function.Name,
			Arguments: unquoteArguments(call.Function.Arguments),
		})
	}

	return parsed
}

// unquoteArguments turns the arguments of a tool call into a JSON object,
// whether the provider sent the object itself or a string holding it. An
// argument list that is neither is passed through as an empty object, so that
// the tool sees no arguments rather than malformed ones.
func unquoteArguments(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage("{}")
	}

	if strings.HasPrefix(trimmed, "\"") {
		var unquoted string
		if err := json.Unmarshal([]byte(trimmed), &unquoted); err != nil {
			return json.RawMessage("{}")
		}

		trimmed = strings.TrimSpace(unquoted)
	}

	if !strings.HasPrefix(trimmed, "{") {
		return json.RawMessage("{}")
	}

	return json.RawMessage(trimmed)
}

// anthropicTools renders tools in the Messages API shape.
func anthropicTools(tools []Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}

	rendered := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		rendered = append(rendered, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": tool.InputSchema,
		})
	}

	return rendered
}

// anthropicMessages renders a conversation for the Messages API, which differs
// from the OpenAI shape in two ways that matter here: roles must alternate, and
// tool results are content blocks of a user turn rather than turns of their
// own. Consecutive tool results are therefore merged into one user message.
func anthropicMessages(messages []Message) []map[string]any {
	rendered := make([]map[string]any, 0, len(messages))
	pendingResults := make([]map[string]any, 0, 4)

	flush := func() {
		if len(pendingResults) == 0 {
			return
		}

		rendered = append(rendered, map[string]any{"role": "user", "content": pendingResults})
		pendingResults = make([]map[string]any, 0, 4)
	}

	for _, message := range messages {
		switch message.Role {
		case RoleSystem:
			// Carried in the top-level system field instead.

		case RoleTool:
			pendingResults = append(pendingResults, map[string]any{
				"type":        anthropicBlockToolResult,
				"tool_use_id": message.ToolCallID,
				"content":     message.Content,
			})

		case RoleUser:
			flush()

			if message.Content == "" {
				continue
			}

			rendered = append(rendered, map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": anthropicBlockText, "text": message.Content}},
			})

		case RoleAssistant:
			flush()

			blocks := make([]map[string]any, 0, len(message.ToolCalls)+1)
			if message.Content != "" {
				blocks = append(blocks, map[string]any{"type": anthropicBlockText, "text": message.Content})
			}

			for _, call := range message.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  anthropicBlockToolUse,
					"id":    call.ID,
					"name":  call.Name,
					"input": json.RawMessage(call.Arguments),
				})
			}

			if len(blocks) == 0 {
				continue
			}

			rendered = append(rendered, map[string]any{"role": "assistant", "content": blocks})
		}
	}

	flush()

	return rendered
}

// Chat asks the model for one answer, offering it tools it may ask to run. The
// conversation is passed whole on every call: this service keeps no state.
//
// The answer is streamed: onDelta is called with each piece of text as it
// arrives, so the chat widget can show the answer being written. Tool calls
// arrive in fragments and are only complete when the stream ends.
func (c *openAICompatibleClient) Chat(ctx context.Context, messages []Message, tools []Tool, onDelta DeltaFunc) (*ChatResult, error) {
	// A refusal arrives as a status before the stream opens, so nothing has
	// been handed to onDelta yet and the whole request can simply be repeated.
	send := func() (io.ReadCloser, error) {
		reqBody := map[string]any{
			"model":    c.conf.Model,
			"messages": openAIMessages(messages, false),
			"stream":   true,
		}
		reqBody[c.maxTokensField()] = chatMaxTokens
		c.setTemperature(reqBody, chatTemperature)

		if rendered := openAITools(tools); rendered != nil {
			reqBody["tools"] = rendered
			reqBody["tool_choice"] = toolChoiceAuto
			c.setToolReasoning(reqBody)
		}

		if !isOfficialOpenAI(c.baseURL) {
			reqBody["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
		}

		req, err := newJSONRequest(ctx, http.MethodPost, c.baseURL+"/chat/completions", reqBody)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Accept", "text/event-stream")

		if c.conf.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.conf.APIKey)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		return streamBody(resp)
	}

	body, err := send()
	for c.retryAfter(err) {
		body, err = send()
	}
	if err != nil {
		return nil, err
	}
	defer body.Close()

	result := &ChatResult{}
	text := &strings.Builder{}
	calls := newToolCallAccumulator()

	err = readSSE(body, func(_ string, data []byte) error {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &chunk); err != nil {
			// A backend may interleave keep-alive or usage payloads that don't
			// match this shape; skipping them is better than failing the answer.
			return nil
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				text.WriteString(choice.Delta.Content)
				emitDelta(onDelta, choice.Delta.Content)
			}

			for _, call := range choice.Delta.ToolCalls {
				calls.add(call.Index, call.ID, call.Function.Name, call.Function.Arguments)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	result.Text = strings.TrimSpace(text.String())
	result.ToolCalls = calls.result()

	return result, nil
}

// Chat asks the model for one answer through the Messages API, streaming the
// text as it is written. Anthropic streams a tool call's arguments as partial
// JSON, which is accumulated per content block.
func (c *anthropicClient) Chat(ctx context.Context, messages []Message, tools []Tool, onDelta DeltaFunc) (*ChatResult, error) {
	reqBody := map[string]any{
		"model":       c.conf.Model,
		"max_tokens":  chatMaxTokens,
		"temperature": chatTemperature,
		"messages":    anthropicMessages(messages),
		"stream":      true,
	}

	if system := systemPrompt(messages); system != "" {
		reqBody["system"] = system
	}

	if rendered := anthropicTools(tools); rendered != nil {
		reqBody["tools"] = rendered
	}

	req, err := newJSONRequest(ctx, http.MethodPost, c.baseURL+"/messages", reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "text/event-stream")

	if c.conf.APIKey != "" {
		req.Header.Set("x-api-key", c.conf.APIKey)
	}
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := streamBody(resp)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	text := &strings.Builder{}
	calls := newToolCallAccumulator()

	err = readSSE(body, func(event string, data []byte) error {
		switch event {
		case "content_block_start":
			var payload struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return nil
			}

			if payload.ContentBlock.Type == anthropicBlockToolUse {
				calls.add(payload.Index, payload.ContentBlock.ID, payload.ContentBlock.Name, "")
			}

		case "content_block_delta":
			var payload struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return nil
			}

			switch payload.Delta.Type {
			case "text_delta":
				if payload.Delta.Text != "" {
					text.WriteString(payload.Delta.Text)
					emitDelta(onDelta, payload.Delta.Text)
				}
			case "input_json_delta":
				calls.add(payload.Index, "", "", payload.Delta.PartialJSON)
			}

		case "error":
			return fmt.Errorf("stream error: %s", truncateForError(string(data)))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ChatResult{Text: strings.TrimSpace(text.String()), ToolCalls: calls.result()}, nil
}

// Chat asks the model for one answer through Ollama's chat endpoint, which
// streams newline-delimited JSON rather than server-sent events. Ollama sends a
// tool call whole, so nothing has to be reassembled.
func (c *ollamaClient) Chat(ctx context.Context, messages []Message, tools []Tool, onDelta DeltaFunc) (*ChatResult, error) {
	reqBody := map[string]any{
		"model":    c.conf.Model,
		"messages": openAIMessages(messages, true),
		"stream":   true,
		"options": map[string]any{
			"temperature": chatTemperature,
			"num_predict": chatMaxTokens,
		},
	}

	if rendered := openAITools(tools); rendered != nil {
		reqBody["tools"] = rendered
	}

	req, err := newJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/chat", reqBody)
	if err != nil {
		return nil, err
	}

	if c.conf.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.conf.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := streamBody(resp)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	text := &strings.Builder{}
	var toolCalls []ToolCall

	err = readJSONLines(body, func(data []byte) error {
		var chunk struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []openAIToolCall `json:"tool_calls"`
			} `json:"message"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil
		}

		if chunk.Error != "" {
			return fmt.Errorf("stream error: %s", truncateForError(chunk.Error))
		}

		if chunk.Message.Content != "" {
			text.WriteString(chunk.Message.Content)
			emitDelta(onDelta, chunk.Message.Content)
		}

		toolCalls = append(toolCalls, parseOpenAIToolCalls(chunk.Message.ToolCalls)...)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ChatResult{Text: strings.TrimSpace(text.String()), ToolCalls: toolCalls}, nil
}
