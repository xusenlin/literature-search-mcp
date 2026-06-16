# literature-mcp

A small **MCP server** (stdio transport) that searches academic literature across
**PubMed**, **Semantic Scholar**, **arXiv**, and **CQVIP** (维普), and returns
the URL plus the fields you actually need to write a paper — title, authors,
year, venue, DOI, abstract, and (where applicable) citation count and PDF link.

Built with the official Go SDK: `github.com/modelcontextprotocol/go-sdk/mcp`.

## Tools

| Tool | Description |
|---|---|
| `search_pubmed` | Search PubMed by keyword. Uses ESearch + EFetch, so abstracts are included. |
| `search_semantic_scholar` | Search Semantic Scholar (all fields). Returns citation counts. |
| `search_arxiv` | Search arXiv (preprints in physics, math, CS, …). Returns PDF link. |
| `search_cqvip` | Search CQVIP 维普 literature by keyword. Paid per call; use explicitly only when CQVIP Chinese literature is needed. Searches the last 5 years with a fixed page size of 50 and supports language filtering. |
| `search_all` | Run PubMed, Semantic Scholar, and arXiv in parallel for the same query. **Per-platform failures are non-fatal** — successful platforms still contribute their results, failures are reported under `errors`. CQVIP is intentionally excluded because it is paid per call. |

### Input schema

Single-platform tools (PubMed, S2, arXiv) take:

```json
{ "query": "string",  "limit": 10 }
```

All search tools automatically restrict results to the most recent 5 years. They
do not expose year range parameters.

`search_cqvip` is a paid per-call backend and always requests 50 records from
CQVIP. It supports an optional language filter:

```json
{
  "query":        "新能源",
  "language":     "zh"
}
```

`search_all` takes:

```json
{ "query": "string",  "limit_per_source": 5 }
```

`search_all` covers PubMed, Semantic Scholar, and arXiv only. It never calls
CQVIP; use `search_cqvip` explicitly when paid CQVIP coverage is required.

### Output shape

```json
{
  "query":   "...",
  "total":   12,
  "sources": ["pubmed", "arxiv"],          // search_all only
  "errors":  ["semantic_scholar: ..."],    // search_all only, may be empty
  "papers": [
    {
      "source":         "pubmed | semantic_scholar | arxiv | cqvip",
      "id":             "PMID / S2 paperId / arXiv id / CQVIP id",
      "title":          "...",
      "authors":        ["..."],
      "year":           "2024",
      "venue":          "Nature",
      "doi":            "10.1038/...",
      "url":            "https://pubmed.ncbi.nlm.nih.gov/12345/",
      "pdf_url":        "https://arxiv.org/pdf/...",   // arXiv only
      "abstract":       "...",
      "citation_count": 42                              // Semantic Scholar only
    }
  ]
}
```

## Configuration

All API keys are **optional**. Free platforms (PubMed, Semantic Scholar, arXiv)
will work without keys, just with stricter rate limits. CQVIP is paid-only and
charged per call — without `CQVIP_API_KEY` set, `search_cqvip` returns a clear
error. `search_all` never calls CQVIP.

| Variable | Used for | Notes |
|---|---|---|
| `PUBMED_API_KEY` | PubMed E-utilities | Optional. Without one: 3 req/s; with one: 10 req/s. Get one at <https://www.ncbi.nlm.nih.gov/account/>. |
| `SEMANTIC_SCHOLAR_API_KEY` | Semantic Scholar Graph API | Optional. Without one: shared public rate limit. Apply at <https://www.semanticscholar.org/product/api>. |
| `CQVIP_API_KEY` | CQVIP unified search API | Sent as `Authorization: Bearer <key>`. Required for the CQVIP backend; the rest of the server still works without it. Uses the dedicated `adv-search-jkhh` endpoint with fixed `size=50`. |
| `LITERATURE_MCP_TOOL` | Identifies your client to NCBI | Defaults to `literature-mcp`. |
| `LITERATURE_MCP_CONTACT` | Email NCBI uses to contact you about issues | Optional but recommended by NCBI. |

arXiv requires no key.

## Build

```bash
go build -o literature-mcp .
```

Requires Go 1.24+ (matches the SDK requirement).

## Use with Claude Desktop / any MCP client

```json
{
  "mcpServers": {
    "literature": {
      "command": "/absolute/path/to/literature-mcp",
      "env": {
        "PUBMED_API_KEY":           "...",
        "SEMANTIC_SCHOLAR_API_KEY": "...",
        "CQVIP_API_KEY":            "..."
      }
    }
  }
}
```

Any of the `env` entries can be omitted.

## Project layout

```
literature-mcp/
├── go.mod
├── main.go             // server + tool registration
├── types.go            // Paper / SearchResult types (drive the schemas)
├── config.go           // env-var loading
├── http.go             // shared HTTP client (GET + POST JSON helpers)
├── pubmed.go           // ESearch + EFetch (XML)
├── semanticscholar.go  // graph/v1/paper/search (JSON)
├── arxiv.go            // export.arxiv.org/api/query (Atom XML)
└── cqvip.go            // CQVIP literature search (JSON, Bearer auth)
```
