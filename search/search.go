package search

import "context"

// Result represents a single search result.
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	FetchedURL  string `json:"fetched_url,omitempty"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content,omitempty"`
}

// Searcher is the interface for search backends.
type Searcher interface {
	Search(ctx context.Context, query string, limit int) ([]Result, error)
}
