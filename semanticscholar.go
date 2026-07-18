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
	Total int       `json:"total"`
	Data  []ssPaper `json:"data"`
}

type ssPaper struct {
	PaperID       string           `json:"paperId"`
	Title         string           `json:"title"`
	Abstract      string           `json:"abstract"`
	Year          int              `json:"year"`
	Venue         string           `json:"venue"`
	URL           string           `json:"url"`
	CitationCount int              `json:"citationCount"`
	Authors       []ssAuthor       `json:"authors"`
	ExternalIDs   map[string]any   `json:"externalIds"`
	OpenAccessPDF *ssOpenAccessPDF `json:"openAccessPdf"`
}

type ssOpenAccessPDF struct {
	URL    string `json:"url"`
	Status string `json:"status"`
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
	originalLimit := limit
	fetchLimit := limit * 5
	if fetchLimit < limit {
		fetchLimit = limit
	}
	if fetchLimit > 100 {
		fetchLimit = 100 // S2 hard cap on a single page.
	}

	yearFrom, _ := recentPublicationYearRange(time.Now())

	q := url.Values{}
	q.Set("query", query)
	q.Set("limit", fmt.Sprintf("%d", fetchLimit))
	q.Set("fields", "title,abstract,year,venue,url,citationCount,authors,externalIds,openAccessPdf")
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
		paper := ssPaperToPaper(p)
		if paper.PDFURL == "" {
			continue
		}
		papers = append(papers, paper)
		if len(papers) >= originalLimit {
			break
		}
	}
	return papers, len(papers), nil
}

// ssPaperToPaper converts one Semantic Scholar record into the unified Paper
// shape. The same conversion serves the search path and the by-id detail path.
func ssPaperToPaper(p ssPaper) Paper {
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

	return Paper{
		Source:        "semantic_scholar",
		ID:            p.PaperID,
		Title:         p.Title,
		Authors:       authors,
		Year:          year,
		Venue:         p.Venue,
		DOI:           doi,
		URL:           paperURL,
		PDFURL:        semanticScholarPDFURL(p),
		Abstract:      p.Abstract,
		CitationCount: p.CitationCount,
	}
}

func semanticScholarPDFURL(p ssPaper) string {
	if p.OpenAccessPDF == nil {
		return ""
	}
	return p.OpenAccessPDF.URL
}

const semanticScholarDetailURL = "https://api.semanticscholar.org/graph/v1/paper"

// GetSemanticScholarDetail fetches a single Semantic Scholar paper by paperId
// (also accepts DOI:, PMID:, ARXIV: prefixed ids, which the Graph API resolves).
// The API key is optional; without one the public rate limit applies.
func GetSemanticScholarDetail(ctx context.Context, cfg Config, paperID string) (Paper, error) {
	q := url.Values{}
	q.Set("fields", "title,abstract,year,venue,url,citationCount,authors,externalIds,openAccessPdf")

	headers := map[string]string{}
	if cfg.SemanticScholarAPIKey != "" {
		headers["x-api-key"] = cfg.SemanticScholarAPIKey
	}

	body, err := httpGet(ctx, semanticScholarDetailURL+"/"+url.PathEscape(paperID)+"?"+q.Encode(), headers)
	if err != nil {
		return Paper{}, fmt.Errorf("semantic scholar detail: %w", err)
	}

	var p ssPaper
	if err := json.Unmarshal(body, &p); err != nil {
		return Paper{}, fmt.Errorf("semantic scholar detail decode: %w", err)
	}
	if p.PaperID == "" && p.Title == "" {
		return Paper{}, fmt.Errorf("paper not found: semantic_scholar %s", paperID)
	}
	paper := ssPaperToPaper(p)
	if paper.PDFURL != "" {
		fullText, err := extractPDFTextFromURL(ctx, paper.PDFURL, nil)
		if err != nil {
			setFullTextError(&paper, "semantic scholar pdf extraction failed: %v", err)
		} else {
			applyFullText(&paper, fullText)
		}
	} else {
		setFullTextError(&paper, "no full text content available")
	}
	return paper, nil
}
