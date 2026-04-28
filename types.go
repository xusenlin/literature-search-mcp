package main

// Paper is the unified paper record returned by every search backend.
// Fields are kept lean — only what's useful for paper writing.
type Paper struct {
	Source        string   `json:"source" jsonschema:"which platform the result came from: pubmed | semantic_scholar | arxiv | cqvip"`
	ID            string   `json:"id" jsonschema:"native identifier on the source platform (PMID, S2 paperId, arXiv id, CQVIP id)"`
	Title         string   `json:"title" jsonschema:"paper title"`
	Authors       []string `json:"authors,omitempty" jsonschema:"list of author names"`
	Year          string   `json:"year,omitempty" jsonschema:"publication year"`
	Venue         string   `json:"venue,omitempty" jsonschema:"journal or conference name"`
	DOI           string   `json:"doi,omitempty" jsonschema:"DOI if available"`
	URL           string   `json:"url" jsonschema:"canonical URL to the paper page"`
	PDFURL        string   `json:"pdf_url,omitempty" jsonschema:"direct PDF link if available (mainly arXiv)"`
	Abstract      string   `json:"abstract,omitempty" jsonschema:"paper abstract"`
	CitationCount int      `json:"citation_count,omitempty" jsonschema:"number of citations (Semantic Scholar only)"`
}

// SearchResult is what a single tool call returns.
type SearchResult struct {
	Query   string   `json:"query" jsonschema:"the keyword query that was executed"`
	Total   int      `json:"total" jsonschema:"number of papers returned"`
	Papers  []Paper  `json:"papers" jsonschema:"the matching papers"`
	Errors  []string `json:"errors,omitempty" jsonschema:"non-fatal errors per platform (only set for search_all)"`
	Sources []string `json:"sources,omitempty" jsonschema:"platforms that successfully returned results (only for search_all)"`
}
