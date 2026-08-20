// Package assistant runs the product's built-in chat assistant: a loop between
// a model of a configured AI integration and the tools MCP Server exposes over
// the Model Context Protocol. The tools that only read are called as the model
// asks for them; the ones that change the product are never called until the
// user has approved that exact call (see confirm.go).
package assistant

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/build"
)

const (
	// mcpClientTimeout bounds a single MCP request. The agent loop has a
	// deadline of its own; this one keeps a wedged tool from eating all of it.
	mcpClientTimeout = 60 * time.Second

	// maxToolResultBytes bounds what one tool result contributes to the next
	// prompt. MCP Server already compacts its answers; this is the backstop
	// that keeps a large one from crowding out the conversation.
	maxToolResultBytes = 16 * 1024

	toolResultTruncationMarker = "\n...[truncated]"

	authorizationHeader = "Authorization"
)

// ToolBox is the set of tools an assistant run may use. It is an interface so
// that the agent loop can be tested without an MCP server.
type ToolBox interface {
	// ListTools returns the tools the model may ask for.
	ListTools(ctx context.Context) ([]ai.Tool, error)
	// CallTool runs one tool and returns its result as text for the model. A
	// tool that fails returns an error; the loop reports it to the model rather
	// than giving up.
	CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	// Close releases the session.
	Close() error
}

// ToolBoxFactory opens a tool session for one chat. The caller's own
// authorization travels with it, so that the tools run under the permissions of
// the person asking, and the audit of the services behind MCP Server names them
// rather than the notifier.
type ToolBoxFactory interface {
	Open(ctx context.Context, authorization string) (ToolBox, error)
}

// MCPToolBoxFactory connects to MCP Server over Streamable HTTP.
type MCPToolBoxFactory struct {
	endpoint  string
	transport http.RoundTripper
}

// NewMCPToolBoxFactory builds a factory for the MCP server at endpoint.
func NewMCPToolBoxFactory(endpoint string, tlsConfig *tls.Config) *MCPToolBoxFactory {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	// MCP Server is an internal service; a proxy configured for outbound
	// traffic has no business intercepting this.
	transport.Proxy = nil

	return &MCPToolBoxFactory{endpoint: strings.TrimRight(endpoint, "/"), transport: transport}
}

// Open connects to MCP Server and initialises a session.
func (f *MCPToolBoxFactory) Open(ctx context.Context, authorization string) (ToolBox, error) {
	httpClient := &http.Client{
		Timeout:   mcpClientTimeout,
		Transport: &authTransport{base: f.transport, authorization: authorization},
	}

	client := mcp.NewClient(&mcp.Implementation{Name: build.AppName + "-assistant", Version: build.Release}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   f.endpoint,
		HTTPClient: httpClient,
		// MCP Server runs stateless, so there is no server-initiated stream to
		// listen on and a GET would only be answered with 405.
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("can't connect to mcp server: %w", err)
	}

	return &mcpToolBox{session: session}, nil
}

// authTransport adds the caller's authorization to every MCP request.
type authTransport struct {
	base          http.RoundTripper
	authorization string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.authorization != "" && req.Header.Get(authorizationHeader) == "" {
		// The request must not be mutated in place: RoundTrippers are shared.
		clone := req.Clone(req.Context())
		clone.Header.Set(authorizationHeader, t.authorization)
		req = clone
	}

	return t.base.RoundTrip(req)
}

type mcpToolBox struct {
	session *mcp.ClientSession
}

func (b *mcpToolBox) ListTools(ctx context.Context) ([]ai.Tool, error) {
	resp, err := b.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("can't list mcp tools: %w", err)
	}

	tools := make([]ai.Tool, 0, len(resp.Tools))
	for _, tool := range resp.Tools {
		schema, err := toolSchema(tool.InputSchema)
		if err != nil {
			// A tool whose schema can't be rendered can't be offered safely:
			// the model would be guessing at its arguments.
			continue
		}

		tools = append(tools, ai.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: schema,
			Title:       title(tool),
			// A server that says nothing about a tool is taken to offer one
			// that changes something: the loop then asks the user before
			// running it, which is the safe way to be wrong.
			ReadOnly:    tool.Annotations != nil && tool.Annotations.ReadOnlyHint,
			Destructive: tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint,
		})
	}

	return tools, nil
}

func (b *mcpToolBox) CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	resp, err := b.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: json.RawMessage(arguments),
	})
	if err != nil {
		return "", fmt.Errorf("can't call tool %s: %w", name, err)
	}

	text := renderToolResult(resp)

	if resp.IsError {
		return text, fmt.Errorf("tool %s failed: %s", name, text)
	}

	return text, nil
}

func (b *mcpToolBox) Close() error {
	return b.session.Close()
}

// title is what the user is shown when asked to approve a call: the tool's own
// title when the server gave one, its name otherwise.
func title(tool *mcp.Tool) string {
	if tool.Annotations != nil && tool.Annotations.Title != "" {
		return tool.Annotations.Title
	}

	if tool.Title != "" {
		return tool.Title
	}

	return tool.Name
}

// toolSchema normalises whatever shape the SDK handed back into the JSON Schema
// object the providers take.
func toolSchema(schema any) (map[string]any, error) {
	if schema == nil {
		return map[string]any{"type": "object"}, nil
	}

	if typed, ok := schema.(map[string]any); ok {
		return typed, nil
	}

	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}

	return decoded, nil
}

// renderToolResult turns a tool result into the text the model reads. The
// structured content is preferred: it is the compact JSON MCP Server built for
// exactly this purpose.
func renderToolResult(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}

	if result.StructuredContent != nil {
		if encoded, err := json.Marshal(result.StructuredContent); err == nil {
			return truncateResult(string(encoded))
		}
	}

	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}

	return truncateResult(strings.Join(parts, "\n"))
}

func truncateResult(text string) string {
	if len(text) <= maxToolResultBytes {
		return text
	}

	cut := maxToolResultBytes
	for cut > 0 && text[cut]&0xC0 == 0x80 {
		cut--
	}

	return text[:cut] + toolResultTruncationMarker
}
