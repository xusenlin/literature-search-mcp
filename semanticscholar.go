package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

const semanticScholarSearchURL = "https://api.semanticscholar.org/graph/v1/paper/search"

type ssSearchResp struct {
	Total int        `json:"total"`
	Data  []ssPaper `json:"data"`
}

type ssPaper struct {
	PaperID       string     `json:"paperId"`
	Title         string     `json:"title"`
	Abstract      string     `json:"abstract"`
	Year          int        `json:"year"`
	Venue         string     `json:"venue"`
	URL           string     `json:"url"`
	CitationCount int        `json:"citationCount"`
	Authors       []ssAuthor `json:"authors"`
	ExternalIDs   map[string]any `json:"externalIds"`
}

type ssAuthor struct {
	AuthorID string `json:"authorId"`
	Name     string `json:"name"`
}

// SearchSemanticScholar runs a keyword search against the Semantic Scholar
// graph API. The API key is optional; without one the public rate limit
// applies (currently 1 req/s globally).
func SearchSemanticScholar(ctx context.Context, cfg Config, query string, limit int) ([]Paper, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100 // S2 hard cap on a single page.
	}

	yearFrom := time.Now().Year() - 5

	q := url.Values{}
	q.Set("query", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("fields", "title,abstract,year,venue,url,citationCount,authors,externalIds")
	q.Set("year", fmt.Sprintf("%d-", yearFrom))

	headers := map[string]string{}
	if cfg.SemanticScholarAPIKey != "" {
		headers["x-api-key"] = cfg.SemanticScholarAPIKey
	}

	body, err := httpGet(ctx, semanticScholarSearchURL+"?"+q.Encode(), headers)
	if err != nil {
		return nil, 0, fmt.Errorf("semantic scholar search: %w", err)
	}

	var sr ssSearchResp
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, 0, fmt.Errorf("semantic scholar decode: %w", err)
	}

	papers := make([]Paper, 0, len(sr.Data))
	for _, p := range sr.Data {
		authors := make([]string, 0, len(p.Authors))
		for _, a := range p.Authors {
			if a.Name != "" {
				authors = append(authors, a.Name)
			}
		}

		// DOI lives under externalIds.DOI (string).
		doi := ""
		if p.ExternalIDs != nil {
			if v, ok := p.ExternalIDs["DOI"].(string); ok {
				doi = v
			}
		}

		paperURL := p.URL
		if paperURL == "" && p.PaperID != "" {
			paperURL = "https://www.semanticscholar.org/paper/" + p.PaperID
		}

		year := ""
		if p.Year > 0 {
			year = fmt.Sprintf("%d", p.Year)
		}

		papers = append(papers, Paper{
			Source:        "semantic_scholar",
			ID:            p.PaperID,
			Title:         p.Title,
			Authors:       authors,
			Year:          year,
			Venue:         p.Venue,
			DOI:           doi,
			URL:           paperURL,
			Abstract:      p.Abstract,
			CitationCount: p.CitationCount,
		})
	}
	return papers, sr.Total, nil
}
