package main

// Paper is the unified paper record returned by every search backend.
// Fields are kept lean — only what's useful for paper writing.
type Paper struct {
	Source            string   `json:"source" jsonschema:"which platform the result came from: pubmed | semantic_scholar | arxiv | cqvip"`
	ID                string   `json:"id" jsonschema:"该平台的原生 id: pubmed=PMID(纯数字) | semantic_scholar=paperId(40位十六进制) | arxiv=arXiv id(如 2106.09685) | cqvip=维普期刊文献id(10位数字，仅来自 search_cqvip 返回的可下载期刊文献)"`
	Title             string   `json:"title" jsonschema:"paper title"`
	Authors           []string `json:"authors,omitempty" jsonschema:"list of author names"`
	Year              string   `json:"year,omitempty" jsonschema:"publication year"`
	Venue             string   `json:"venue,omitempty" jsonschema:"journal or conference name"`
	DOI               string   `json:"doi,omitempty" jsonschema:"DOI if available"`
	URL               string   `json:"url" jsonschema:"canonical URL to the paper page"`
	PDFURL            string   `json:"pdf_url,omitempty" jsonschema:"direct PDF link if available (mainly arXiv)"`
	Abstract          string   `json:"abstract,omitempty" jsonschema:"paper abstract"`
	FullText          string   `json:"-"`
	FullTextSource    string   `json:"-"`
	FullTextTruncated bool     `json:"-"`
	FullTextError     string   `json:"-"`
	CitationCount     int      `json:"citation_count,omitempty" jsonschema:"number of citations (Semantic Scholar only)"`
}

// SearchResult is what a single tool call returns.
type SearchResult struct {
	Query        string         `json:"query" jsonschema:"the keyword query that was executed"`
	Total        int            `json:"total" jsonschema:"total number of results returned"`
	Papers       []Paper        `json:"papers" jsonschema:"the matching papers"`
	Errors       []string       `json:"errors,omitempty" jsonschema:"non-fatal errors per platform (only set for search_all)"`
	Sources      []string       `json:"sources,omitempty" jsonschema:"platforms that successfully returned results (only for search_all)"`
	SourceTotals map[string]int `json:"source_totals,omitempty" jsonschema:"per-platform total match count (only for search_all)"`
}
