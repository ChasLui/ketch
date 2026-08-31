package mcp

import (
	"context"
	"errors"

	"github.com/1broseidon/ketch/code"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// CodeInput is the input schema for the "code" tool.
type CodeInput struct {
	Query   string `json:"query" jsonschema:"the code search query"`
	Backend string `json:"backend,omitempty" jsonschema:"code search backend: grepapp, sourcegraph, github, or firecrawl (default: the configured backend)"`
	Lang    string `json:"lang,omitempty" jsonschema:"language filter appended to the query; on firecrawl it scopes to GitHub repos and drops documentation results"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max number of results (default: the configured limit)"`
	Regexp  bool   `json:"regexp,omitempty" jsonschema:"interpret query as a regular expression (grepapp, sourcegraph only)"`
}

// CodeOutput is the output schema for the "code" tool. Results carries the
// same result objects as the CLI's `ketch code --json` (which emits them as
// a bare array; MCP structured content needs the object wrapper).
type CodeOutput struct {
	Results []code.Result `json:"results"`
}

func (s *Server) registerCodeTool() {
	mcpsdk.AddTool(s.mcp, &mcpsdk.Tool{
		Name: "code",
		Description: "Search code across open-source repositories using Grep (mcp.grep.app), Sourcegraph, GitHub Code Search, or the Firecrawl Developer Index (semantic retrieval over issues, merged PRs, READMEs and curated docs) (default: the configured backend)." +
			errTaxonomy,
		Annotations: readOnlyOpenWorld(),
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in CodeInput) (*mcpsdk.CallToolResult, CodeOutput, error) {
		if in.Query == "" {
			return nil, CodeOutput{}, errf(kindValidation, "query is required")
		}
		backend := in.Backend
		if backend == "" {
			backend = s.cfg.CodeBackend
		}
		limit := in.Limit
		if limit <= 0 {
			limit = s.cfg.Limit
		}

		searcher, err := code.NewFromConfig(s.cfg, backend)
		if err != nil {
			return nil, CodeOutput{}, backendErrf(err, code.ErrUnknownBackend)
		}

		results, err := searcher.Search(ctx, code.Query{
			Term:   in.Query,
			Lang:   in.Lang,
			Limit:  limit,
			Regexp: in.Regexp,
		})
		if err != nil {
			if errors.Is(err, code.ErrRegexpUnsupported) {
				return nil, CodeOutput{}, errf(kindValidation, "backend %q does not support regexp (try backend=grepapp or backend=sourcegraph)", backend)
			}
			return nil, CodeOutput{}, upstreamErrf(err, "code search failed")
		}

		return nil, CodeOutput{Results: results}, nil
	})
}
