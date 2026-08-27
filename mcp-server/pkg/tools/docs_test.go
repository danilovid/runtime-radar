package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/docs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSearchDocs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "quickstart.md"), []byte("# Quickstart\n\nRuntime Radar kills a pod when a rule says so.\n"), 0o600); err != nil {
		t.Fatalf("can't write the fixture: %v", err)
	}

	index, err := docs.Load(root)
	if err != nil {
		t.Fatalf("can't load the index: %v", err)
	}

	deps := newTestDeps(t, nil, nil, nil)
	deps.Docs = index

	result, err := searchDocs(deps)(context.Background(), SearchDocsArgs{Query: "kills a pod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 || result.Matches[0].Path != "quickstart.md" {
		t.Fatalf("matches = %+v", result.Matches)
	}
	if result.Indexed != 1 {
		t.Errorf("indexed = %d, want 1", result.Indexed)
	}

	if _, err := searchDocs(deps)(context.Background(), SearchDocsArgs{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("error = %v, want InvalidArgument for an empty query", err)
	}

	// A query that finds nothing returns an empty list rather than null, so
	// that a client does not have to handle both.
	empty, err := searchDocs(deps)(context.Background(), SearchDocsArgs{Query: "wireguard"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if empty.Matches == nil || empty.Count != 0 {
		t.Errorf("matches = %v, want an empty list", empty.Matches)
	}
}
