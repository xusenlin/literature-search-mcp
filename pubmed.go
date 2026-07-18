package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	pubmedESearchURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi"
	pubmedEFetchURL  = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"
	pubmedArticleURL = "https://pubmed.ncbi.nlm.nih.gov/"
	pmcArticleURL    = "https://pmc.ncbi.nlm.nih.gov/articles/"
)

// --- ESearch JSON shape (only what we need) ---

type pubmedESearchResp struct {
	ESearchResult struct {
		Count  string   `json:"count"`
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

// --- EFetch XML shape ---

type pubmedArticleSet struct {
	XMLName  xml.Name        `xml:"PubmedArticleSet"`
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	MedlineCitation struct {
		PMID    string `xml:"PMID"`
		Article struct {
			ArticleTitle    string `xml:"ArticleTitle"`
			VernacularTitle string `xml:"VernacularTitle"`
			Abstract        struct {
				Texts []pubmedAbstractText `xml:"AbstractText"`
			} `xml:"Abstract"`
			AuthorList struct {
				Authors []struct {
					LastName       string `xml:"LastName"`
					ForeName       string `xml:"ForeName"`
					CollectiveName string `xml:"CollectiveName"`
				} `xml:"Author"`
			} `xml:"AuthorList"`
			Journal struct {
				Title        string `xml:"Title"`
				JournalIssue struct {
					PubDate struct {
						Year    string `xml:"Year"`
						MedDate string `xml:"MedlineDate"`
					} `xml:"PubDate"`
				} `xml:"JournalIssue"`
			} `xml:"Journal"`
			ELocationIDs []pubmedELocation `xml:"ELocationID"`
		} `xml:"Article"`
	} `xml:"MedlineCitation"`
	PubmedData struct {
		ArticleIDList struct {
			ArticleIDs []pubmedArticleID `xml:"ArticleId"`
		} `xml:"ArticleIdList"`
	} `xml:"PubmedData"`
}

type pubmedAbstractText struct {
	Label string `xml:"Label,attr"`
	Value string `xml:",chardata"`
}

type pubmedELocation struct {
	Type  string `xml:"EIdType,attr"`
	Value string `xml:",chardata"`
}

type pubmedArticleID struct {
	Type  string `xml:"IdType,attr"`
	Value string `xml:",chardata"`
}

// SearchPubMed runs a keyword search against PubMed.
//
// Workflow:
//  1. ESearch (JSON) → list of PMIDs.
//  2. EFetch  (XML)  → full records including abstracts and direct PMC ids.
//
// The API key is optional; without one, NCBI rate-limits to 3 req/s.
func SearchPubMed(ctx context.Context, cfg Config, query string, limit int) ([]Paper, int, error) {
	if limit <= 0 {
		limit = 20
	}
	originalLimit := limit
	fetchLimit := limit * 5
	if fetchLimit < limit {
		fetchLimit = limit
	}
	if fetchLimit > 100 {
		fetchLimit = 100
	}

	yearFrom, yearTo := recentPublicationYearRange(time.Now())

	// 1) ESearch — find PMIDs.
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("term", query)
	q.Set("retmode", "json")
	q.Set("retmax", fmt.Sprintf("%d", fetchLimit))
	q.Set("sort", "relevance")
	q.Set("mindate", fmt.Sprintf("%d", yearFrom))
	q.Set("maxdate", fmt.Sprintf("%d", yearTo))
	q.Set("datetype", "pdat")
	if cfg.Tool != "" {
		q.Set("tool", cfg.Tool)
	}
	if cfg.Contact != "" {
		q.Set("email", cfg.Contact)
	}
	if cfg.PubMedAPIKey != "" {
		q.Set("api_key", cfg.PubMedAPIKey)
	}

	body, err := httpGet(ctx, pubmedESearchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("pubmed esearch: %w", err)
	}

	var sr pubmedESearchResp
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, 0, fmt.Errorf("pubmed esearch decode: %w", err)
	}
	if len(sr.ESearchResult.IDList) == 0 {
		return []Paper{}, 0, nil
	}

	// 2) EFetch — full records.
	fq := url.Values{}
	fq.Set("db", "pubmed")
	fq.Set("id", strings.Join(sr.ESearchResult.IDList, ","))
	fq.Set("retmode", "xml")
	if cfg.Tool != "" {
		fq.Set("tool", cfg.Tool)
	}
	if cfg.Contact != "" {
		fq.Set("email", cfg.Contact)
	}
	if cfg.PubMedAPIKey != "" {
		fq.Set("api_key", cfg.PubMedAPIKey)
	}

	xmlBody, err := httpGet(ctx, pubmedEFetchURL+"?"+fq.Encode(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("pubmed efetch: %w", err)
	}

	var set pubmedArticleSet
	if err := xml.Unmarshal(xmlBody, &set); err != nil {
		return nil, 0, fmt.Errorf("pubmed efetch decode: %w", err)
	}

	papers := make([]Paper, 0, len(set.Articles))
	for _, a := range set.Articles {
		p, ok := pubmedArticleToPaper(a)
		if !ok {
			continue
		}
		if pubmedArticlePMCID(a) == "" {
			continue
		}
		papers = append(papers, p)
		if len(papers) >= originalLimit {
			break
		}
	}
	return papers, len(papers), nil
}

// pubmedArticlePMCID returns the article's own PMC identifier from the
// top-level PubmedData/ArticleIdList populated by EFetch. Reading it here
// avoids a slow ELink request per search result and cannot confuse cited PMC
// records with the article itself.
func pubmedArticlePMCID(a pubmedArticle) string {
	for _, id := range a.PubmedData.ArticleIDList.ArticleIDs {
		if !strings.EqualFold(id.Type, "pmc") {
			continue
		}
		pmcid := strings.ToUpper(strings.TrimSpace(id.Value))
		if pmcid == "" {
			continue
		}
		if strings.HasPrefix(pmcid, "PMC") {
			return pmcid
		}
		return "PMC" + pmcid
	}
	return ""
}

// pubmedArticleToPaper converts one EFetch article into the unified Paper shape.
// Returns ok=false when the record has no usable title and should be skipped.
func pubmedArticleToPaper(a pubmedArticle) (Paper, bool) {
	mc := a.MedlineCitation
	art := mc.Article

	// Authors.
	authors := make([]string, 0, len(art.AuthorList.Authors))
	for _, au := range art.AuthorList.Authors {
		switch {
		case au.CollectiveName != "":
			authors = append(authors, au.CollectiveName)
		case au.ForeName != "" && au.LastName != "":
			authors = append(authors, au.ForeName+" "+au.LastName)
		case au.LastName != "":
			authors = append(authors, au.LastName)
		}
	}

	// Abstract — concatenate labelled sections.
	var ab strings.Builder
	for i, t := range art.Abstract.Texts {
		if i > 0 {
			ab.WriteString("\n\n")
		}
		if t.Label != "" {
			ab.WriteString(t.Label + ": ")
		}
		ab.WriteString(strings.TrimSpace(t.Value))
	}

	// Year.
	year := art.Journal.JournalIssue.PubDate.Year
	if year == "" && art.Journal.JournalIssue.PubDate.MedDate != "" {
		// MedlineDate looks like "2020 Jan-Feb"; pull leading 4 digits.
		md := art.Journal.JournalIssue.PubDate.MedDate
		if len(md) >= 4 {
			year = md[:4]
		}
	}

	// DOI — prefer ELocationID, fall back to ArticleIdList.
	doi := ""
	for _, e := range art.ELocationIDs {
		if strings.EqualFold(e.Type, "doi") {
			doi = strings.TrimSpace(e.Value)
			break
		}
	}
	if doi == "" {
		for _, id := range a.PubmedData.ArticleIDList.ArticleIDs {
			if strings.EqualFold(id.Type, "doi") {
				doi = strings.TrimSpace(id.Value)
				break
			}
		}
	}

	// Title — prefer ArticleTitle, fall back to VernacularTitle.
	title := strings.TrimSpace(art.ArticleTitle)
	if title == "" || title == "[Not Available]." {
		title = strings.TrimSpace(art.VernacularTitle)
	}
	if title == "" || title == "[Not Available]." {
		return Paper{}, false
	}

	return Paper{
		Source:   "pubmed",
		ID:       mc.PMID,
		Title:    title,
		Authors:  authors,
		Year:     year,
		Venue:    strings.TrimSpace(art.Journal.Title),
		DOI:      doi,
		URL:      pubmedArticleURL + mc.PMID + "/",
		Abstract: strings.TrimSpace(ab.String()),
	}, true
}

// GetPubMedDetail fetches a single PubMed record by PMID via EFetch. Unlike the
// search path, no year filter is applied — a PMID always points at one record.
func GetPubMedDetail(ctx context.Context, cfg Config, pmid string) (Paper, error) {
	fq := url.Values{}
	fq.Set("db", "pubmed")
	fq.Set("id", pmid)
	fq.Set("retmode", "xml")
	if cfg.Tool != "" {
		fq.Set("tool", cfg.Tool)
	}
	if cfg.Contact != "" {
		fq.Set("email", cfg.Contact)
	}
	if cfg.PubMedAPIKey != "" {
		fq.Set("api_key", cfg.PubMedAPIKey)
	}

	xmlBody, err := httpGet(ctx, pubmedEFetchURL+"?"+fq.Encode(), nil)
	if err != nil {
		return Paper{}, fmt.Errorf("pubmed efetch: %w", err)
	}

	var set pubmedArticleSet
	if err := xml.Unmarshal(xmlBody, &set); err != nil {
		return Paper{}, fmt.Errorf("pubmed efetch decode: %w", err)
	}
	if len(set.Articles) == 0 {
		return Paper{}, fmt.Errorf("paper not found: pubmed %s", pmid)
	}

	p, ok := pubmedArticleToPaper(set.Articles[0])
	if !ok {
		return Paper{}, fmt.Errorf("paper not found: pubmed %s", pmid)
	}
	pmcid := pubmedArticlePMCID(set.Articles[0])
	if pmcid == "" {
		setFullTextError(&p, "no full text content available")
		return p, nil
	}

	fullText, err := fetchPMCFullText(ctx, cfg, pmcid)
	if err != nil {
		setFullTextError(&p, "pubmed pmc full-text fetch failed: %v", err)
		return p, nil
	}
	p.URL = pmcArticleURL + pmcid + "/"
	applyFullText(&p, fullText)
	return p, nil
}
func fetchPMCFullText(ctx context.Context, cfg Config, pmcid string) (fullTextResult, error) {
	q := url.Values{}
	q.Set("db", "pmc")
	q.Set("id", pmcid)
	q.Set("retmode", "xml")
	if cfg.Tool != "" {
		q.Set("tool", cfg.Tool)
	}
	if cfg.Contact != "" {
		q.Set("email", cfg.Contact)
	}
	if cfg.PubMedAPIKey != "" {
		q.Set("api_key", cfg.PubMedAPIKey)
	}

	body, err := httpGet(ctx, pubmedEFetchURL+"?"+q.Encode(), nil)
	if err != nil {
		return fullTextResult{}, err
	}
	return extractPMCBodyText(body)
}
