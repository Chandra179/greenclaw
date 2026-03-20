package scraper

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"greenclaw/internal/browser"
	"greenclaw/internal/config"
	"greenclaw/internal/fetcher"
	"greenclaw/internal/router"
	"greenclaw/internal/store"
	"greenclaw/internal/youtube"
)

// ResultStore abstracts result storage for testability.
type ResultStore interface {
	Put(r *store.Result)
	Get(url string) (*store.Result, bool)
	All() []*store.Result
	Count() int
}

// BrowserFetcher abstracts browser-based page fetching for testability.
type BrowserFetcher interface {
	FetchPage(ctx context.Context, url string) (*store.Result, error)
	Close()
}

// Scraper orchestrates URL processing with concurrency control.
type Scraper struct {
	cfg           config.Config
	client        *http.Client
	store         ResultStore
	browserPool   BrowserFetcher
	httpSem       chan struct{}
	browserSem    chan struct{}
	ytPipelineCfg youtube.PipelineConfig
}

// New creates a Scraper with the given configuration.
func New(cfg config.Config) *Scraper {
	return &Scraper{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		store:       store.New(),
		browserPool: browser.NewPool(cfg.RecycleAfter),
		httpSem:     make(chan struct{}, cfg.HTTPConcurrency),
		browserSem:  make(chan struct{}, cfg.BrowserConcurrency),
		ytPipelineCfg: youtube.PipelineConfig{
			ExtractTranscripts: cfg.YouTube.ExtractTranscripts,
			TranscriptLangs:    cfg.YouTube.TranscriptLangs,
			DownloadAudio:      cfg.YouTube.DownloadAudio,
			AudioOutputDir:     cfg.YouTube.AudioOutputDir,
			ExportSubtitles:    cfg.YouTube.ExportSubtitles,
			SubtitleFormats:    cfg.YouTube.SubtitleFormats,
			SubtitleOutputDir:  cfg.YouTube.SubtitleOutputDir,
			TranscribeAudio:    cfg.YouTube.TranscribeAudio,
			TranscriberModel:    cfg.Transcriber.Model,
			TranscriberModelDir: cfg.Transcriber.ModelDir,
			TranscriberLanguage: cfg.Transcriber.Language,
		},
	}
}

// NewWithDeps creates a Scraper with injected dependencies for testing.
func NewWithDeps(cfg config.Config, client *http.Client, rs ResultStore, bp BrowserFetcher) *Scraper {
	return &Scraper{
		cfg:         cfg,
		client:      client,
		store:       rs,
		browserPool: bp,
		httpSem:     make(chan struct{}, cfg.HTTPConcurrency),
		browserSem:  make(chan struct{}, cfg.BrowserConcurrency),
	}
}

// Run processes all URLs concurrently and returns the store with results.
func (s *Scraper) Run(ctx context.Context, urls []string) ResultStore {
	var wg sync.WaitGroup

	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			s.processURL(ctx, url)
		}(u)
	}

	wg.Wait()
	return s.store
}

func (s *Scraper) processURL(ctx context.Context, url string) {
	var lastErr error

	for attempt := range s.cfg.RetryAttempts {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("[retry] %s attempt %d/%d, waiting %v", url, attempt+1, s.cfg.RetryAttempts, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				s.storeError(url, ctx.Err())
				return
			}
		}

		result, err := s.fetchURL(ctx, url)
		if err != nil {
			lastErr = err
			log.Printf("[error] %s: %v", url, err)
			continue
		}

		s.store.Put(result)
		log.Printf("[done] %s (stage %d)", url, result.Stage)
		return
	}

	s.storeError(url, lastErr)
}

func (s *Scraper) fetchURL(ctx context.Context, url string) (*store.Result, error) {
	// Acquire HTTP semaphore for classification
	select {
	case s.httpSem <- struct{}{}:
		defer func() { <-s.httpSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Short-circuit YouTube URLs — no HEAD request needed
	if ytType, id, ok := router.IsYouTube(url); ok {
		log.Printf("[router] %s → youtube (%d, %s)", url, ytType, id)
		pipeline := youtube.NewPipeline(youtube.NewClient(s.client), s.ytPipelineCfg)
		return pipeline.Process(ctx, url, ytType, id)
	}

	ct, err := router.Classify(ctx, s.client, url)
	if err != nil {
		// If HEAD fails, assume HTML and try fetching anyway
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

	case store.ContentHTML:
		return s.fetchHTML(ctx, url)

	default:
		return fetcher.FetchHTML(ctx, s.client, url)
	}
}

func (s *Scraper) fetchHTML(ctx context.Context, url string) (*store.Result, error) {
	// Stage 1: plain HTTP
	result, err := fetcher.FetchHTML(ctx, s.client, url)
	if err == nil {
		return result, nil
	}

	if !errors.Is(err, fetcher.ErrNeedsEscalation) {
		return nil, err
	}

	log.Printf("[escalate] %s → browser", url)

	// Release HTTP semaphore before acquiring browser semaphore
	// (already released via defer in fetchURL, so we just acquire browser sem)
	select {
	case s.browserSem <- struct{}{}:
		defer func() { <-s.browserSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return s.browserPool.FetchPage(ctx, url)
}

func (s *Scraper) storeError(url string, err error) {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	s.store.Put(&store.Result{
		URL:       url,
		Error:     errMsg,
		FetchedAt: time.Now(),
	})
}

// Close cleans up resources.
func (s *Scraper) Close() {
	s.browserPool.Close()
}
