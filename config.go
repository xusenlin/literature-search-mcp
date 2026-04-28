package main

import "os"

// Config holds optional API keys for each platform.
// All keys are optional — services work without them, just with stricter rate
// limits (or in CQVIP's case, the platform is simply skipped).
type Config struct {
	PubMedAPIKey          string
	SemanticScholarAPIKey string
	CQVIPAPIKey           string
	// arXiv has no API key.

	// Tool / contact info recommended by NCBI for E-utilities.
	Tool    string
	Contact string
}

func LoadConfig() Config {
	return Config{
		PubMedAPIKey:          os.Getenv("PUBMED_API_KEY"),
		SemanticScholarAPIKey: os.Getenv("SEMANTIC_SCHOLAR_API_KEY"),
		CQVIPAPIKey:           os.Getenv("CQVIP_API_KEY"),
		Tool:                  envOr("LITERATURE_MCP_TOOL", "literature-mcp"),
		Contact:               os.Getenv("LITERATURE_MCP_CONTACT"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
