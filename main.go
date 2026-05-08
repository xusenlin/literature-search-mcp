package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Tool input shapes -------------------------------------------------------

type SingleSearchInput struct {
	Query string `json:"query" jsonschema:"keyword(s) to search; supports the platform's native query syntax"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results to return (default 10)"`
}

// CQVIPSearchInput exposes the extra filters the CQVIP advanced API supports.
type CQVIPSearchInput struct {
	Query       string `json:"query" jsonschema:"keyword(s) to search"`
	Limit       int    `json:"limit,omitempty" jsonschema:"max results to return (default 10)"`
	OnlyPDF     bool   `json:"only_pdf,omitempty" jsonschema:"restrict to records that have a downloadable PDF"`
	OnlyOA      bool   `json:"only_oa,omitempty" jsonschema:"restrict to open-access records"`
	Language    string `json:"language,omitempty" jsonschema:"paper language filter, e.g. 'zh' or 'en'"`
	SearchField string `json:"search_field,omitempty" jsonschema:"CQVIP search-field code; defaults to 'U' (all fields)"`
}

type AllSearchInput struct {
	Query          string `json:"query" jsonschema:"keyword(s) to search across every enabled platform"`
	LimitPerSource int    `json:"limit_per_source,omitempty" jsonschema:"max results per platform (default 5)"`
}

// --- Tool handlers -----------------------------------------------------------

// nonNil returns ps as-is, or a non-nil empty slice if ps is nil. The SDK
// validates output against the inferred schema, which says `papers` must be
// an array; a nil slice would serialize to `null` and fail validation.
func nonNil(ps []Paper) []Paper {
	if ps == nil {
		return []Paper{}
	}
	return ps
}

func makePubMedHandler(cfg Config) func(context.Context, *mcp.CallToolRequest, SingleSearchInput) (*mcp.CallToolResult, SearchResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in SingleSearchInput) (*mcp.CallToolResult, SearchResult, error) {
		papers, total, err := SearchPubMed(ctx, cfg, in.Query, in.Limit)
		if err != nil {
			return nil, SearchResult{}, err
		}
		papers = nonNil(papers)
		return nil, SearchResult{
			Query:  in.Query,
			Total:  total,
			Papers: papers,
		}, nil
	}
}

func makeSemanticScholarHandler(cfg Config) func(context.Context, *mcp.CallToolRequest, SingleSearchInput) (*mcp.CallToolResult, SearchResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in SingleSearchInput) (*mcp.CallToolResult, SearchResult, error) {
		papers, total, err := SearchSemanticScholar(ctx, cfg, in.Query, in.Limit)
		if err != nil {
			return nil, SearchResult{}, err
		}
		papers = nonNil(papers)
		return nil, SearchResult{
			Query:  in.Query,
			Total:  total,
			Papers: papers,
		}, nil
	}
}

func makeArxivHandler(cfg Config) func(context.Context, *mcp.CallToolRequest, SingleSearchInput) (*mcp.CallToolResult, SearchResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in SingleSearchInput) (*mcp.CallToolResult, SearchResult, error) {
		papers, total, err := SearchArxiv(ctx, cfg, in.Query, in.Limit)
		if err != nil {
			return nil, SearchResult{}, err
		}
		papers = nonNil(papers)
		return nil, SearchResult{
			Query:  in.Query,
			Total:  total,
			Papers: papers,
		}, nil
	}
}

func makeCQVIPHandler(cfg Config) func(context.Context, *mcp.CallToolRequest, CQVIPSearchInput) (*mcp.CallToolResult, SearchResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in CQVIPSearchInput) (*mcp.CallToolResult, SearchResult, error) {
		papers, total, err := SearchCQVIP(ctx, cfg, in.Query, in.Limit, CQVIPOptions{
			OnlyPDF:     in.OnlyPDF,
			OnlyOA:      in.OnlyOA,
			Language:    in.Language,
			SearchField: in.SearchField,
		})
		if err != nil {
			return nil, SearchResult{}, err
		}
		papers = nonNil(papers)
		return nil, SearchResult{
			Query:  in.Query,
			Total:  total,
			Papers: papers,
		}, nil
	}
}

// makeAllHandler runs every backend in parallel. A failing backend does not
// fail the whole call — it's recorded under `errors` and we move on. CQVIP
// is only included if its API key is configured (it has no free tier).
func makeAllHandler(cfg Config) func(context.Context, *mcp.CallToolRequest, AllSearchInput) (*mcp.CallToolResult, SearchResult, error) {
	type backend struct {
		name string
		fn   func(context.Context, Config, string, int) ([]Paper, int, error)
	}
	backends := []backend{
		{"pubmed", SearchPubMed},
		{"semantic_scholar", SearchSemanticScholar},
		{"arxiv", SearchArxiv},
	}
	if cfg.CQVIPAPIKey != "" {
		backends = append(backends, backend{
			name: "cqvip",
			fn: func(ctx context.Context, c Config, q string, n int) ([]Paper, int, error) {
				return SearchCQVIP(ctx, c, q, n, CQVIPOptions{})
			},
		})
	}

	return func(ctx context.Context, _ *mcp.CallToolRequest, in AllSearchInput) (*mcp.CallToolResult, SearchResult, error) {
		limit := in.LimitPerSource
		if limit <= 0 {
			limit = 10
		}

		var (
			mu           sync.Mutex
			papers       = []Paper{}
			errs         []string
			sources      []string
			sourceTotals = make(map[string]int)
			wg           sync.WaitGroup
		)

		for _, b := range backends {
			b := b
			wg.Add(1)
			go func() {
				defer wg.Done()
				ps, total, err := b.fn(ctx, cfg, in.Query, limit)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", b.name, err))
					return
				}
				papers = append(papers, ps...)
				sources = append(sources, b.name)
				sourceTotals[b.name] = total
			}()
		}
		wg.Wait()

		return nil, SearchResult{
			Query:        in.Query,
			Total:        len(papers),
			Papers:       papers,
			Errors:       errs,
			Sources:      sources,
			SourceTotals: sourceTotals,
		}, nil
	}
}

// --- main --------------------------------------------------------------------

func main() {
	cfg := LoadConfig()

	// Logs go to stderr — stdout is reserved for the JSON-RPC stream.
	log.SetOutput(os.Stderr)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "literature-mcp",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "search_pubmed",
		Description: "Search PubMed by keyword (restricted to last 5 years). Returns papers with title, " +
			"authors, year, journal, DOI, URL, and abstract. Useful for biomedical literature.",
	}, makePubMedHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name: "search_semantic_scholar",
		Description: "Search Semantic Scholar by keyword (restricted to last 5 years). Covers all fields " +
			"and returns title, authors, year, venue, DOI, URL, abstract, and citation count.",
	}, makeSemanticScholarHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name: "search_arxiv",
		Description: "Search arXiv by keyword (restricted to last 5 years). Returns title, authors, year, " +
			"journal reference (if any), DOI, abstract page URL, PDF URL, and abstract. " +
			"Useful for physics, math, and CS preprints.",
	}, makeArxivHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name: "search_cqvip",
		Description: "Search CQVIP (维普) Chinese academic database by keyword " +
			"(restricted to last 5 years, max 10 results per request). " +
			"Note: the 'total' field reflects returned page size, NOT the full match count. " +
			"Supports open-access only, has-PDF only, and language filters. " +
			"Returns title, authors, year, journal, DOI, URL, and abstract. " +
			"Requires CQVIP_API_KEY (paid service).",
	}, makeCQVIPHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name: "search_all",
		Description: "Search PubMed, Semantic Scholar, and arXiv (plus CQVIP if its " +
			"key is configured) in parallel for the same query " +
			"(restricted to last 5 years). Per-platform failures " +
			"are reported under `errors` and do not fail the call; results from " +
			"successful platforms are merged. " +
			"The `source_totals` field shows per-platform match counts; " +
			"note that CQVIP's total may reflect only the current page size.",
	}, makeAllHandler(cfg))

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
