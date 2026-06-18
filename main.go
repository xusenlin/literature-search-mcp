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

type DetailInput struct {
	Source string `json:"source" jsonschema:"平台，取值之一: pubmed | semantic_scholar | arxiv | cqvip"`
	ID     string `json:"id"     jsonschema:"该平台的原生 id，必须与 source 配对、来自同一条搜索结果: pubmed→PMID(纯数字,如 42309480); semantic_scholar→paperId(40位十六进制); arxiv→arXiv id(如 2106.09685,可带版本号); cqvip→维普期刊文献id(10位数字，只能使用 search_cqvip 返回的可下载期刊文献 id)"`
}

type DetailResult struct {
	Content string `json:"content" jsonschema:"paper full text content when available"`
	Msg     string `json:"msg" jsonschema:"OK on success, otherwise the reason content could not be returned"`
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

// makeDetailHandler fetches readable paper content by source + id.
func makeDetailHandler(cfg Config) func(context.Context, *mcp.CallToolRequest, DetailInput) (*mcp.CallToolResult, DetailResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in DetailInput) (*mcp.CallToolResult, DetailResult, error) {
		var (
			p   Paper
			err error
		)
		switch in.Source {
		case "pubmed":
			p, err = GetPubMedDetail(ctx, cfg, in.ID)
		case "semantic_scholar":
			p, err = GetSemanticScholarDetail(ctx, cfg, in.ID)
		case "arxiv":
			p, err = GetArxivDetail(ctx, in.ID)
		case "cqvip":
			p, err = GetCQVIPDetail(ctx, cfg, in.ID)
		default:
			return nil, DetailResult{Msg: fmt.Sprintf("unsupported source %q", in.Source)}, nil
		}
		if err != nil {
			return nil, DetailResult{Msg: err.Error()}, nil
		}
		if p.FullText != "" {
			return nil, DetailResult{Content: p.FullText, Msg: "OK"}, nil
		}
		if p.FullTextError != "" {
			return nil, DetailResult{Msg: p.FullTextError}, nil
		}
		return nil, DetailResult{Msg: "no full text content available"}, nil
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
		Description: "搜索 PubMed 中可获取正文的文献，返回标题、作者、年份、来源、DOI、摘要和 PMID。输入 query、limit。",
	}, makePubMedHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_semantic_scholar",
		Description: "搜索 Semantic Scholar 中有开放 PDF 的文献，返回标题、作者、年份、来源、DOI、摘要、引用数和 paperId。输入 query、limit。",
	}, makeSemanticScholarHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_arxiv",
		Description: "搜索 arXiv 中有 PDF 的预印本文献，返回标题、作者、年份、摘要、arXiv id 和 PDF 链接。输入 query、limit。",
	}, makeArxivHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_cqvip",
		Description: "搜索维普中文可下载期刊文献，返回标题、作者、年份、期刊、摘要和期刊文献id。按次计费，输入 query，可选 language。",
	}, makeCQVIPHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_all",
		Description: "并行搜索 PubMed、Semantic Scholar 和 arXiv 中可获取正文或 PDF 的文献，不包含维普。输入 query、limit_per_source。",
	}, makeAllHandler(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_paper_detail",
		Description: "根据搜索结果的 source 和 id 获取正文内容。返回 content 和 msg；content 为空时看 msg。输入 source、id。",
	}, makeDetailHandler(cfg))

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
