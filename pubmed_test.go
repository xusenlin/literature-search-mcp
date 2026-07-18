package main

import (
	"encoding/xml"
	"testing"
)

func TestPubmedArticlePMCID(t *testing.T) {
	tests := []struct {
		name string
		xml  string
		want string
	}{
		{
			name: "direct pmc id",
			xml: `<PubmedArticleSet><PubmedArticle>
				<MedlineCitation><PMID>38476451</PMID><Article><ArticleTitle>Title</ArticleTitle></Article></MedlineCitation>
				<PubmedData><ArticleIdList>
					<ArticleId IdType="pubmed">38476451</ArticleId>
					<ArticleId IdType="pmc">PMC10928249</ArticleId>
				</ArticleIdList></PubmedData>
			</PubmedArticle></PubmedArticleSet>`,
			want: "PMC10928249",
		},
		{
			name: "normalizes numeric pmc id",
			xml: `<PubmedArticleSet><PubmedArticle>
				<MedlineCitation><PMID>1</PMID><Article><ArticleTitle>Title</ArticleTitle></Article></MedlineCitation>
				<PubmedData><ArticleIdList><ArticleId IdType="pmc"> 12345 </ArticleId></ArticleIdList></PubmedData>
			</PubmedArticle></PubmedArticleSet>`,
			want: "PMC12345",
		},
		{
			name: "ignores pmc id from cited reference",
			xml: `<PubmedArticleSet><PubmedArticle>
				<MedlineCitation><PMID>2</PMID><Article><ArticleTitle>Title</ArticleTitle></Article></MedlineCitation>
				<PubmedData><ArticleIdList><ArticleId IdType="pubmed">2</ArticleId></ArticleIdList>
					<ReferenceList><Reference><ArticleIdList><ArticleId IdType="pmc">PMC999</ArticleId></ArticleIdList></Reference></ReferenceList>
				</PubmedData>
			</PubmedArticle></PubmedArticleSet>`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var set pubmedArticleSet
			if err := xml.Unmarshal([]byte(tt.xml), &set); err != nil {
				t.Fatalf("xml.Unmarshal() error = %v", err)
			}
			if len(set.Articles) != 1 {
				t.Fatalf("got %d articles, want 1", len(set.Articles))
			}
			if got := pubmedArticlePMCID(set.Articles[0]); got != tt.want {
				t.Fatalf("pubmedArticlePMCID() = %q, want %q", got, tt.want)
			}
		})
	}
}
