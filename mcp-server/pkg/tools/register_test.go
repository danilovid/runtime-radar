package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/client"
)

// TestRegister checks that every tool is exposed with a schema the SDK can
// infer (AddTool panics otherwise) and with the untrusted-data warning in its
// description, which is what stops a model from acting on telemetry.
func TestRegister(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	// The tools managing API tokens are only offered when Public API is
	// configured, and this asserts the whole set.
	deps.PublicAPI = client.NewPublicAPI("http://public-api:9000", nil)

	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-server", Version: "v0.0.0"}, nil)
	Register(server, deps)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("can't connect the server: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("can't connect the client: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("can't list tools: %v", err)
	}

	// The value is whether the tool only reads. A tool that changes the
	// product must say so in its annotation, because that is what an MCP
	// client decides whether to ask the user about.
	readOnlyByTool := map[string]bool{
		"search_runtime_events":   true,
		"get_runtime_event":       true,
		"get_process_context":     true,
		"list_detectors":          true,
		"get_runtime_stats":       true,
		"list_rules":              true,
		"list_api_tokens":         true,
		"search_docs":             true,
		"list_admission_sources":  true,
		"search_admission_events": true,
		"get_admission_event":     true,
		"create_rule":             false,
		"delete_rule":             false,
		"create_api_token":        false,
		"delete_api_token":        false,
		"set_admission_source":    false,
		"create_admission_source": false,
	}

	// The tools that belong to one half of the product, and the half they belong to.
	scopeByTool := map[string]string{
		"search_runtime_events":   string(auth.ScopeRuntimeMonitor),
		"get_runtime_event":       string(auth.ScopeRuntimeMonitor),
		"get_process_context":     string(auth.ScopeRuntimeMonitor),
		"list_detectors":          string(auth.ScopeRuntimeMonitor),
		"get_runtime_stats":       string(auth.ScopeRuntimeMonitor),
		"list_admission_sources":  string(auth.ScopeAdmission),
		"search_admission_events": string(auth.ScopeAdmission),
		"get_admission_event":     string(auth.ScopeAdmission),
		"set_admission_source":    string(auth.ScopeAdmission),
		"create_admission_source": string(auth.ScopeAdmission),
	}

	found := map[string]bool{}

	for _, tool := range result.Tools {
		wantReadOnly, ok := readOnlyByTool[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		found[tool.Name] = true

		if !strings.Contains(tool.Description, "UNTRUSTED TELEMETRY") {
			t.Errorf("tool %q does not warn about untrusted data", tool.Name)
		}

		// A tool of one half of the product says so, which is what lets a
		// first-party client offer only the half its configuration allows.
		if wantScope, scoped := scopeByTool[tool.Name]; scoped {
			if got := tool.Meta[auth.ScopeMetaKey]; got != wantScope {
				t.Errorf("tool %q publishes scope %v, expected %q", tool.Name, got, wantScope)
			}
		} else if _, published := tool.Meta[auth.ScopeMetaKey]; published {
			t.Errorf("tool %q publishes a scope but belongs to neither half", tool.Name)
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q has no annotations", tool.Name)
			continue
		}
		if tool.Annotations.ReadOnlyHint != wantReadOnly {
			t.Errorf("tool %q: read-only hint is %v, want %v", tool.Name, tool.Annotations.ReadOnlyHint, wantReadOnly)
		}
		if !wantReadOnly && !strings.Contains(tool.Description, "THIS TOOL CHANGES RUNTIME RADAR") {
			t.Errorf("tool %q does not say that it changes the product", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", tool.Name)
		}
	}

	for name := range readOnlyByTool {
		if !found[name] {
			t.Errorf("tool %q was not registered", name)
		}
	}
}

// TestRegisterWithoutPublicAPI checks that the tools managing API tokens are
// left out when there is no Public API to manage them through: a tool that
// cannot work is worse than a missing one, because a model will still try it.
func TestRegisterWithoutPublicAPI(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})

	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-server", Version: "v0.0.0"}, nil)
	Register(server, deps)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("can't connect the server: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("can't connect the client: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("can't list tools: %v", err)
	}

	for _, tool := range result.Tools {
		if strings.HasSuffix(tool.Name, "_api_token") || strings.HasSuffix(tool.Name, "_api_tokens") {
			t.Errorf("tool %q is offered without a public api to reach", tool.Name)
		}
	}
}

// TestCallToolThroughSession exercises the whole path a client takes: JSON-RPC
// call, argument decoding, the handler, and the structured result.
func TestCallToolThroughSession(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})

	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-server", Version: "v0.0.0"}, nil)
	Register(server, deps)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("can't connect the server: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("can't connect the client: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_runtime_events",
		Arguments: map[string]any{"namespace": []string{"prod"}, "limit": 5},
	})
	if err != nil {
		t.Fatalf("can't call the tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool returned an error: %+v", result.Content)
	}
	if result.StructuredContent == nil {
		t.Fatal("tool returned no structured content")
	}

	// An unparsable timestamp must come back as a tool error the model can
	// correct, not as a transport failure.
	bad, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_runtime_events",
		Arguments: map[string]any{"since": "yesterday"},
	})
	if err != nil {
		t.Fatalf("can't call the tool: %v", err)
	}
	if !bad.IsError {
		t.Error("an invalid timestamp was accepted")
	}
}
