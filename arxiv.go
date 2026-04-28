package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

const arxivQueryURL = "http://export.arxiv.org/api/query"

// arXiv returns Atom 1.0 with a couple of arxiv:* extensions.
type arxivFeed struct {
	XMLName xml.Name      `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Summary   string        `xml:"summary"`
	Published string        `xml:"published"`
	Authors   []arxivAuthor `xml:"author"`
	Links     []arxivLink   `xml:"link"`

	// arxiv:* extensions — encoding/xml matches by local name.
	DOI        string `xml:"doi"`
	JournalRef string `xml:"journal_ref"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

// SearchArxiv runs a keyword search against arXiv's public Atom API.
// arXiv requires no API key.
func SearchArxiv(ctx context.Context, _ Config, query string, limit int) ([]Paper, error) {
	if limit <= 0 {
		limit = 10
	}

	q := url.Values{}
	q.Set("search_query", "all:"+query)
	q.Set("start", "0")
	q.Set("max_results", fmt.Sprintf("%d", limit))
	q.Set("sortBy", "relevance")
	q.Set("sortOrder", "descending")

	body, err := httpGet(ctx, arxivQueryURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("arxiv search: %w", err)
	}

	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("arxiv decode: %w", err)
	}

	papers := make([]Paper, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		// Authors.
		authors := make([]string, 0, len(e.Authors))
		for _, a := range e.Authors {
			if name := strings.TrimSpace(a.Name); name != "" {
				authors = append(authors, name)
			}
		}

		// Pick the alternate (HTML) link as the canonical URL, and the PDF
		// link if one is advertised. Fall back to the entry id.
		htmlURL := strings.TrimSpace(e.ID)
		pdfURL := ""
		for _, l := range e.Links {
			if l.Rel == "alternate" && l.Type == "text/html" && l.Href != "" {
				htmlURL = l.Href
			}
			if l.Title == "pdf" || l.Type == "application/pdf" {
				pdfURL = l.Href
			}
		}

		// Pull out the arXiv id from the URL (everything after /abs/).
		id := htmlURL
		if i := strings.Index(htmlURL, "/abs/"); i >= 0 {
			id = htmlURL[i+len("/abs/"):]
		}

		// Year from the published date (RFC3339-ish: 2021-06-08T17:23:20Z).
		year := ""
		if len(e.Published) >= 4 {
			year = e.Published[:4]
		}

		venue := strings.TrimSpace(e.JournalRef)
		if venue == "" {
			venue = "arXiv"
		}

		papers = append(papers, Paper{
			Source:   "arxiv",
			ID:       id,
			Title:    cleanWhitespace(e.Title),
			Authors:  authors,
			Year:     year,
			Venue:    venue,
			DOI:      strings.TrimSpace(e.DOI),
			URL:      htmlURL,
			PDFURL:   pdfURL,
			Abstract: cleanWhitespace(e.Summary),
		})
	}
	return papers, nil
}

// arXiv inserts newlines and indentation inside titles/abstracts. Collapse them.
func cleanWhitespace(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join(strings.Fields(s), " ")
}
