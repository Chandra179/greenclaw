package scraper

import (
	"context"
	"errors"
	"log"
	"net/http"

	"greenclaw/internal/browser"
	"greenclaw/internal/config"
	"greenclaw/internal/fetcher"
	"greenclaw/internal/router"
	"greenclaw/internal/store"
)

// BrowserFetcher abstracts browser-based page fetching for testability.
type BrowserFetcher interface {
	FetchPage(ctx context.Context, url string) (*store.Result, error)
	Close()
}

// Scraper handles web content fetching (HTML, JSON, XML, binary).
// It has no knowledge of YouTube or any other platform-specific logic.
type Scraper struct {
	client      *http.Client
	browserPool BrowserFetcher
	browserSem  chan struct{}
}

func New(cfg config.Config) *Scraper {
	return NewWithDeps(cfg, &http.Client{Timeout: cfg.Timeout}, browser.NewPool(cfg.RecycleAfter))
}

// NewWithDeps creates a Scraper with injected dependencies for testing.
func NewWithDeps(cfg config.Config, client *http.Client, bp BrowserFetcher) *Scraper {
	return &Scraper{
		client:      client,
		browserPool: bp,
		browserSem:  make(chan struct{}, cfg.BrowserConcurrency),
	}
}

// Fetch classifies and fetches the given URL, escalating to browser if needed.
// Callers are responsible for overall concurrency control.
func (s *Scraper) Fetch(ctx context.Context, url string) (*store.Result, error) {
	ct, err := router.Classify(ctx, s.client, url)
	if err != nil {
		log.Printf("[router] HEAD failed for %s, assuming HTML: %v", url, err)
		ct = store.ContentHTML
	}

	log.Printf("[router] %s → %s", url, ct)

	switch ct {
	case store.ContentBinary:
		return fetcher.DownloadBinary(ctx, s.client, url, "downloads")
	case store.ContentJSON:
		return fetcher.FetchJSON(ctx, s.client, url)
	case store.ContentXML:
		return fetcher.FetchXML(ctx, s.client, url)
	default:
		return s.fetchHTML(ctx, url)
	}
}

func (s *Scraper) fetchHTML(ctx context.Context, url string) (*store.Result, error) {
	result, err := fetcher.FetchHTML(ctx, s.client, url)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, fetcher.ErrNeedsEscalation) {
		return nil, err
	}

	log.Printf("[escalate] %s → browser", url)

	select {
	case s.browserSem <- struct{}{}:
		defer func() { <-s.browserSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return s.browserPool.FetchPage(ctx, url)
}

// Close cleans up resources.
func (s *Scraper) Close() {
	s.browserPool.Close()
}
