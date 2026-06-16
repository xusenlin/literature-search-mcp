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

type CQVIPSearchInput struct {
	Query    string `json:"query" jsonschema:"keyword(s) to search"`
	Language string `json:"language,omitempty" jsonschema:"paper language filter, e.g. 'zh' or 'en'"`
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
		papers, total, err := SearchCQVIP(ctx, cfg, in.Query, in.Language)
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
// fail the whole call — it's recorded under `errors` and we move on. CQVIP is
// intentionally excluded because it is charged per call.
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
		Name:        "search_pubmed",
		Description: "搜索最近 5 年 PubMed 生物医学文献。适合医学、生命科学、临床相关问题。输入 query 和 limit。",
	}, makePubMedHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_semantic_scholar",
		Description: "搜索最近 5 年 Semantic Scholar 多学科文献。适合通用学术检索，并返回引用次数。输入 query 和 limit。",
	}, makeSemanticScholarHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_arxiv",
		Description: "搜索最近 5 年 arXiv 预印本文献。适合计算机、数学、物理等领域，并返回 PDF 链接。输入 query 和 limit。",
	}, makeArxivHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_cqvip",
		Description: "搜索最近 5 年 CQVIP（维普）中文文献。按调用次数收费，每次固定请求 50 条；不要试探性或重复调用。应先使用免费综合检索，不足时再用本工具补充。输入 query，可选 language。",
	}, makeCQVIPHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_all",
		Description: "并行搜索最近 5 年 PubMed、Semantic Scholar 和 arXiv。适合先做免费综合检索。不会调用 CQVIP；需要维普时必须单独调用 search_cqvip。输入 query 和 limit_per_source。",
	}, makeAllHandler(cfg))

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
