package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	cqvipAdvSearchURL = "https://superapi.cqvip.com/unifiedsearch/search/v1/paper/adv-search"
	cqvipArticleURL   = "https://qikan.cqvip.com/Qikan/Article/Detail?id=" // canonical CQVIP detail page
)

// CQVIPOptions captures the optional filters supported by the advanced
// search endpoint. Zero values mean "do not constrain".
type CQVIPOptions struct {
	YearStart   int    // e.g. 2021
	YearEnd     int    // e.g. 2025
	OnlyPDF     bool   // restrict to records with PDF
	OnlyOA      bool   // restrict to open-access records
	Language    string // "zh" / "en"
	SearchField string // CQVIP field code; defaults to "U" (universal)
}

// --- request body ---

type cqvipReq struct {
	Page        int    `json:"page"`
	Size        int    `json:"size"`
	SearchField string `json:"searchField"`
	Content     string `json:"content"`
	YearStart   int    `json:"yearStart,omitempty"`
	YearEnd     int    `json:"yearEnd,omitempty"`
	PDF         bool   `json:"pdf,omitempty"`
	IsOa        bool   `json:"isOa,omitempty"`
	Language    string `json:"language,omitempty"`
}

// --- response shape ---

type cqvipResp struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    []cqvipPaper `json:"data"`
}

type cqvipPaper struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Abstract    string         `json:"abstr"`
	DOI         string         `json:"doi"`
	Year        string         `json:"year"`
	BeginPage   string         `json:"beginPage"`
	EndPage     string         `json:"endPage"`
	IsOa        bool           `json:"isOa"`
	IsPDF       int            `json:"isPdf"`
	Language    string         `json:"paperLanguage"`
	AuthorInfo  []cqvipNamed   `json:"authorInfo"`
	OrganInfo   []cqvipNamed   `json:"organInfo"`
	KeywordInfo []cqvipNamed   `json:"keywordInfo"`
	Journal     cqvipJournal   `json:"journalInfo"`
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
func SearchCQVIP(ctx context.Context, cfg Config, query string, limit int, opts CQVIPOptions) ([]Paper, error) {
	if cfg.CQVIPAPIKey == "" {
		return nil, errors.New("CQVIP_API_KEY is not set")
	}
	if limit <= 0 {
		limit = 10
	}

	field := opts.SearchField
	if field == "" {
		field = "U"
	}

	body := cqvipReq{
		Page:        1,
		Size:        limit,
		SearchField: field,
		Content:     query,
		YearStart:   opts.YearStart,
		YearEnd:     opts.YearEnd,
		PDF:         opts.OnlyPDF,
		IsOa:        opts.OnlyOA,
		Language:    opts.Language,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + cfg.CQVIPAPIKey,
	}

	raw, err := httpPostJSON(ctx, cqvipAdvSearchURL, body, headers)
	if err != nil {
		return nil, fmt.Errorf("cqvip search: %w", err)
	}

	var resp cqvipResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("cqvip decode: %w", err)
	}
	if resp.Code != 200 {
		msg := resp.Message
		if msg == "" {
			msg = fmt.Sprintf("code %d", resp.Code)
		}
		return nil, fmt.Errorf("cqvip api: %s", msg)
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
	return papers, nil
}
