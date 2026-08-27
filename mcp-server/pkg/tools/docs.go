package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/docs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultDocsLimit = 5
	maxDocsLimit     = 10
)

// SearchDocsArgs are the arguments of search_docs.
type SearchDocsArgs struct {
	Query string `json:"query" jsonschema:"words or a phrase to look for. The documentation is written in Russian and English, so try both when a query finds nothing"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of fragments to return, 1 to 10. Defaults to 5"`
}

// SearchDocsResult is the answer of search_docs.
type SearchDocsResult struct {
	Matches []docs.Match `json:"matches" jsonschema:"relevant documentation fragments, most relevant first"`
	Count   int          `json:"count" jsonschema:"number of fragments returned"`
	Indexed int          `json:"indexed" jsonschema:"number of documentation files the server has indexed"`
}

func registerDocsTools(server *mcp.Server, deps *Deps) {
	// Documentation is shipped inside the image and is the same for everyone,
	// so no product permission gates it. When auth is on, the caller still has
	// to present a valid token.
	addTool(server, deps, &mcp.Tool{
		Name:        "search_docs",
		Annotations: readOnly("Search product documentation"),
		Description: "Full-text search over the Runtime Radar documentation shipped with this server: the " +
			"quickstart, the detector development guide, the use case guide and the help pages. Use it to answer " +
			"questions about how the product works, what a component does or how a feature is configured, instead " +
			"of guessing.",
	}, nil, searchDocs(deps))
}

func searchDocs(deps *Deps) func(context.Context, SearchDocsArgs) (SearchDocsResult, error) {
	return func(_ context.Context, args SearchDocsArgs) (SearchDocsResult, error) {
		if args.Query == "" {
			return SearchDocsResult{}, status.Error(codes.InvalidArgument, "query is empty")
		}

		limit := args.Limit
		switch {
		case limit <= 0:
			limit = defaultDocsLimit
		case limit > maxDocsLimit:
			limit = maxDocsLimit
		}

		matches := deps.Docs.Search(args.Query, limit)
		if matches == nil {
			matches = []docs.Match{}
		}

		return SearchDocsResult{
			Matches: matches,
			Count:   len(matches),
			Indexed: deps.Docs.Len(),
		}, nil
	}
}
