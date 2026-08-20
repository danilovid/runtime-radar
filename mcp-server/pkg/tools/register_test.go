package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRegister checks that every tool is exposed with a schema the SDK can
// infer (AddTool panics otherwise) and with the untrusted-data warning in its
// description, which is what stops a model from acting on telemetry.
func TestRegister(t *testing.T) {
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

	want := map[string]bool{
		"search_runtime_events": false,
		"get_runtime_event":     false,
		"get_process_context":   false,
		"list_detectors":        false,
		"get_runtime_stats":     false,
		"search_docs":           false,
	}

	for _, tool := range result.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		want[tool.Name] = true

		if !strings.Contains(tool.Description, "UNTRUSTED TELEMETRY") {
			t.Errorf("tool %q does not warn about untrusted data", tool.Name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not annotated as read-only", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", tool.Name)
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("tool %q was not registered", name)
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
