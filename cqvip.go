package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	cqvipAdvSearchURL = "https://superapi.cqvip.com/unifiedsearch/search/v1/paper/adv-search-jkhh"
	cqvipArticleURL   = "https://qikan.cqvip.com/Qikan/Article/Detail?id=" // canonical CQVIP detail page
	cqvipPageSize     = 50
)

// --- request body ---

type cqvipReq struct {
	Page        int    `json:"page"`
	Size        int    `json:"size"`
	SearchField string `json:"searchField"`
	Content     string `json:"content"`
	YearStart   int    `json:"yearStart,omitempty"`
	YearEnd     int    `json:"yearEnd,omitempty"`
	Language    string `json:"language,omitempty"`
}

// --- response shape ---

type cqvipResp struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    []cqvipPaper `json:"data"`
}

type cqvipPaper struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Abstract   string       `json:"abstr"`
	DOI        string       `json:"doi"`
	Year       string       `json:"year"`
	AuthorInfo []cqvipNamed `json:"authorInfo"`
	Journal    cqvipJournal `json:"journalInfo"`
}

type cqvipNamed struct {
	Name string `json:"name"`
}

type cqvipJournal struct {
	Name   string `json:"name"`
	Number string `json:"num"`
	Volume string `json:"vol"`
	Year   int    `json:"year"`
}

// SearchCQVIP runs an advanced search against the CQVIP (维普) academic API.
// An API key is REQUIRED — the endpoint always returns 401 without one.
func SearchCQVIP(ctx context.Context, cfg Config, query string, language string) ([]Paper, int, error) {
	if cfg.CQVIPAPIKey == "" {
		return nil, 0, errors.New("CQVIP_API_KEY is not set")
	}

	yearFrom := time.Now().Year() - 5

	body := cqvipReq{
		Page:        1,
		Size:        cqvipPageSize,
		SearchField: "U",
		Content:     query,
		YearStart:   yearFrom,
		YearEnd:     time.Now().Year(),
		Language:    language,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + cfg.CQVIPAPIKey,
	}

	raw, err := httpPostJSON(ctx, cqvipAdvSearchURL, body, headers)
	if err != nil {
		return nil, 0, fmt.Errorf("cqvip search: %w", err)
	}

	var resp cqvipResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, 0, fmt.Errorf("cqvip decode: %w", err)
	}
	if resp.Code != 200 {
		msg := resp.Message
		if msg == "" {
			msg = fmt.Sprintf("code %d", resp.Code)
		}
		return nil, 0, fmt.Errorf("cqvip api: %s", msg)
	}

	papers := make([]Paper, 0, len(resp.Data))
	for _, p := range resp.Data {
		// Authors.
		authors := make([]string, 0, len(p.AuthorInfo))
		for _, a := range p.AuthorInfo {
			if a.Name != "" {
				authors = append(authors, a.Name)
			}
		}

		// Year — prefer the string field, fall back to journalInfo.year.
		year := strings.TrimSpace(p.Year)
		if year == "" && p.Journal.Year > 0 {
			year = fmt.Sprintf("%d", p.Journal.Year)
		}

		papers = append(papers, Paper{
			Source:   "cqvip",
			ID:       p.ID,
			Title:    strings.TrimSpace(p.Title),
			Authors:  authors,
			Year:     year,
			Venue:    strings.TrimSpace(p.Journal.Name),
			DOI:      strings.TrimSpace(p.DOI),
			URL:      cqvipArticleURL + p.ID,
			Abstract: strings.TrimSpace(p.Abstract),
		})
	}
	return papers, len(resp.Data), nil
}
