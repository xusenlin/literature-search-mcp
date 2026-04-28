package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

const (
	pubmedESearchURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi"
	pubmedEFetchURL  = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"
	pubmedArticleURL = "https://pubmed.ncbi.nlm.nih.gov/"
)

// --- ESearch JSON shape (only what we need) ---

type pubmedESearchResp struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

// --- EFetch XML shape ---

type pubmedArticleSet struct {
	XMLName  xml.Name         `xml:"PubmedArticleSet"`
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	MedlineCitation struct {
		PMID    string `xml:"PMID"`
		Article struct {
			ArticleTitle string `xml:"ArticleTitle"`
			Abstract     struct {
				Texts []pubmedAbstractText `xml:"AbstractText"`
			} `xml:"Abstract"`
			AuthorList struct {
				Authors []struct {
					LastName    string `xml:"LastName"`
					ForeName    string `xml:"ForeName"`
					CollectiveName string `xml:"CollectiveName"`
				} `xml:"Author"`
			} `xml:"AuthorList"`
			Journal struct {
				Title        string `xml:"Title"`
				JournalIssue struct {
					PubDate struct {
						Year     string `xml:"Year"`
						MedDate  string `xml:"MedlineDate"`
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
//  2. EFetch  (XML)  → full records including abstracts.
//
// The API key is optional; without one, NCBI rate-limits to 3 req/s.
func SearchPubMed(ctx context.Context, cfg Config, query string, limit int) ([]Paper, error) {
	if limit <= 0 {
		limit = 10
	}

	// 1) ESearch — find PMIDs.
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("term", query)
	q.Set("retmode", "json")
	q.Set("retmax", fmt.Sprintf("%d", limit))
	q.Set("sort", "relevance")
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
		return nil, fmt.Errorf("pubmed esearch: %w", err)
	}

	var sr pubmedESearchResp
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("pubmed esearch decode: %w", err)
	}
	if len(sr.ESearchResult.IDList) == 0 {
		return []Paper{}, nil
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
		return nil, fmt.Errorf("pubmed efetch: %w", err)
	}

	var set pubmedArticleSet
	if err := xml.Unmarshal(xmlBody, &set); err != nil {
		return nil, fmt.Errorf("pubmed efetch decode: %w", err)
	}

	papers := make([]Paper, 0, len(set.Articles))
	for _, a := range set.Articles {
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

		papers = append(papers, Paper{
			Source:   "pubmed",
			ID:       mc.PMID,
			Title:    strings.TrimSpace(art.ArticleTitle),
			Authors:  authors,
			Year:     year,
			Venue:    strings.TrimSpace(art.Journal.Title),
			DOI:      doi,
			URL:      pubmedArticleURL + mc.PMID + "/",
			Abstract: strings.TrimSpace(ab.String()),
		})
	}
	return papers, nil
}
