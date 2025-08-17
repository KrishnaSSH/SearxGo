package engine

import "context"

// Result is the normalized search result returned by all engines.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Engine  string `json:"engine"`
	Favicon string `json:"favicon,omitempty"`
}

// SearchEngine defines a minimal interface for search providers.
type SearchEngine interface {
	Name() string
	Search(ctx context.Context, query string, page int, size int) ([]Result, error)
}
