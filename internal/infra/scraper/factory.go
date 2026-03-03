package scraper

import (
	"context"

	"ai-news/internal/domain"
)

// FetchResult is the data returned by a Fetcher for one article candidate.
type FetchResult struct {
	Title       string
	URL         string
	RemoteThumb string // remote URL for the article thumbnail; may be empty
	RawContent  string
}

// Fetcher retrieves article candidates from a single news source.
type Fetcher interface {
	Fetch(ctx context.Context, src *domain.Source) ([]FetchResult, error)
}

// Factory returns the appropriate Fetcher for a given FetchMethod.
type Factory func(method domain.FetchMethod) Fetcher

// NewFactory builds a Factory that wires up all three fetcher implementations.
// cdpEndpoint is the WebSocket URL for the playwright-chrome container
// (e.g. "ws://playwright-chrome:3000").
func NewFactory(cdpEndpoint string) Factory {
	playwright := NewPlaywrightFetcher(cdpEndpoint)
	return func(method domain.FetchMethod) Fetcher {
		switch method {
		case domain.FetchMethodRSS:
			return &RSSFetcher{}
		case domain.FetchMethodHTTP:
			return &HTTPFetcher{}
		case domain.FetchMethodPlaywright:
			return playwright
		default:
			return &HTTPFetcher{}
		}
	}
}
