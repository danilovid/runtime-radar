package assistant

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func scopedTool(name, scope string) *mcp.Tool {
	tool := &mcp.Tool{Name: name}
	if scope != "" {
		tool.Meta = mcp.Meta{scopeMetaKey: scope}
	}

	return tool
}

func TestToolBoxInScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scopes []string
		tool   *mcp.Tool
		want   bool
	}{
		{
			// An integration created before scopes existed limits nothing.
			name: "no scopes offers everything",
			tool: scopedTool("set_admission_source", "admission"),
			want: true,
		},
		{
			name:   "a tool of the allowed half is offered",
			scopes: []string{"admission"},
			tool:   scopedTool("set_admission_source", "admission"),
			want:   true,
		},
		{
			name:   "a tool of the other half is not",
			scopes: []string{"admission"},
			tool:   scopedTool("search_runtime_events", "runtime_monitor"),
			want:   false,
		},
		{
			// Documentation search and the rules belong to neither half and
			// stay useful whichever one the assistant is limited to.
			name:   "a tool of neither half is always offered",
			scopes: []string{"admission"},
			tool:   scopedTool("search_docs", ""),
			want:   true,
		},
		{
			name:   "both halves may be allowed at once",
			scopes: []string{"admission", "runtime_monitor"},
			tool:   scopedTool("search_runtime_events", "runtime_monitor"),
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			box := &mcpToolBox{scopes: tt.scopes}

			if got := box.inScope(tt.tool); got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestToolBoxRefusesToolOutsideScope covers the model naming a tool it was not
// offered, which it can do from the conversation of an earlier, wider chat.
func TestToolBoxRefusesToolOutsideScope(t *testing.T) {
	t.Parallel()

	box := &mcpToolBox{
		scopes:  []string{"admission"},
		allowed: map[string]bool{"list_admission_sources": true},
	}

	if _, err := box.CallTool(t.Context(), "search_runtime_events", nil); err == nil {
		t.Fatal("expected a tool outside the allowed halves to be refused")
	}
}
