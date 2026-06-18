package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

const (
	maxPDFDownloadBytes = 80 << 20
	maxFullTextRunes    = 120000
)

type fullTextResult struct {
	Text      string
	Source    string
	Truncated bool
}

func extractPDFTextFromURL(ctx context.Context, pdfURL string, headers map[string]string) (fullTextResult, error) {
	if strings.TrimSpace(pdfURL) == "" {
		return fullTextResult{}, fmt.Errorf("empty pdf url")
	}
	body, err := httpGetAccept(ctx, pdfURL, headers, "application/pdf,*/*", maxPDFDownloadBytes)
	if err != nil {
		return fullTextResult{}, err
	}
	return extractPDFText(body)
}

func extractPDFText(body []byte) (fullTextResult, error) {
	r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fullTextResult{}, err
	}
	plain, err := r.GetPlainText()
	if err != nil {
		return fullTextResult{}, err
	}
	text, err := io.ReadAll(plain)
	if err != nil {
		return fullTextResult{}, err
	}
	return makeFullTextResult(string(text), "pdf")
}

func makeFullTextResult(text string, source string) (fullTextResult, error) {
	text = normalizeExtractedText(text)
	if text == "" {
		return fullTextResult{}, fmt.Errorf("no extractable text")
	}
	text, truncated := truncateRunes(text, maxFullTextRunes)
	return fullTextResult{
		Text:      text,
		Source:    source,
		Truncated: truncated,
	}, nil
}

func applyFullText(p *Paper, r fullTextResult) {
	p.FullText = r.Text
	p.FullTextSource = r.Source
	p.FullTextTruncated = r.Truncated
}

func setFullTextError(p *Paper, format string, args ...any) {
	p.FullTextError = fmt.Sprintf(format, args...)
}

func normalizeExtractedText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimFunc(line, unicode.IsSpace)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, strings.Join(strings.Fields(line), " "))
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func truncateRunes(s string, max int) (string, bool) {
	if max <= 0 {
		return s, false
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s, false
	}
	return string(runes[:max]), true
}

func extractPMCBodyText(xmlBody []byte) (fullTextResult, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlBody))
	var b strings.Builder
	bodyDepth := 0
	skipDepth := 0

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fullTextResult{}, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if bodyDepth > 0 {
				bodyDepth++
				if skipDepth > 0 {
					skipDepth++
					continue
				}
				if name == "ref-list" || name == "table-wrap" || name == "fig" || name == "back" {
					skipDepth = 1
					continue
				}
				if name == "title" || name == "p" {
					writeTextBreak(&b)
				}
				continue
			}
			if name == "body" {
				bodyDepth = 1
			}
		case xml.EndElement:
			if bodyDepth > 0 {
				if skipDepth > 0 {
					skipDepth--
				} else if t.Name.Local == "title" || t.Name.Local == "p" || t.Name.Local == "sec" {
					writeTextBreak(&b)
				}
				bodyDepth--
			}
		case xml.CharData:
			if bodyDepth > 0 && skipDepth == 0 {
				text := strings.TrimSpace(string(t))
				if text != "" {
					if b.Len() > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(text)
				}
			}
		}
	}

	return makeFullTextResult(b.String(), "pmc_xml")
}

func writeTextBreak(b *strings.Builder) {
	if b.Len() == 0 {
		return
	}
	s := b.String()
	if strings.HasSuffix(s, "\n\n") {
		return
	}
	if strings.HasSuffix(s, "\n") {
		b.WriteByte('\n')
		return
	}
	b.WriteString("\n\n")
}
