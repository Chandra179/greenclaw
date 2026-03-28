package pipeline

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"greenclaw/internal/browser"
	"greenclaw/internal/config"
	"greenclaw/internal/llm"
	"greenclaw/internal/router"
	"greenclaw/internal/scraper"
	"greenclaw/internal/store"
	"greenclaw/internal/youtube"
	"greenclaw/pkg/transcribe"
	"greenclaw/pkg/ytdl"
)

// ResultStore abstracts result storage for testability.
type ResultStore interface {
	Put(r *store.Result)
	Get(url string) (*store.Result, bool)
	All() []*store.Result
	Count() int
}

// Pipeline orchestrates URL processing, routing to either the web scraper or
// the YouTube processor based on the URL type. It owns concurrency control
// and retry logic; the individual processors handle their own internal concerns.
type Pipeline struct {
	retryAttempts int
	webScraper    *scraper.Scraper
	ytClient      *youtube.Client
	store         ResultStore
	httpSem       chan struct{}
	ytCfg         youtube.PipelineConfig
	transcriber   transcribe.Client
	processor     llm.Client
}

func New(cfg config.Config) *Pipeline {
	httpClient := &http.Client{Timeout: cfg.Timeout}

	var t transcribe.Client
	if cfg.YouTube.TranscribeAudio {
		d, err := time.ParseDuration(cfg.Transcriber.Timeout)
		if err != nil {
			d = 5 * time.Minute
		}
		t = transcribe.NewHTTPClient(cfg.Transcriber.Endpoint, d, cfg.Transcriber.Language)
	}

	var proc llm.Client
	var styles []llm.ProcessingStyle
	if len(cfg.LLM.ProcessingStyles) > 0 {
		d, err := time.ParseDuration(cfg.LLM.Timeout)
		if err != nil {
			d = 60 * time.Second
		}
		proc = llm.NewOllamaClient(cfg.LLM.Endpoint, cfg.LLM.Model, d, cfg.LLM.NumCtx, cfg.LLM.OverlapTokens, cfg.LLM.CacheDir)
		for _, s := range cfg.LLM.ProcessingStyles {
			styles = append(styles, llm.ProcessingStyle(s))
		}
	}

	return &Pipeline{
		retryAttempts: cfg.RetryAttempts,
		webScraper:    scraper.NewWithDeps(cfg, httpClient, browser.NewPool(cfg.RecycleAfter)),
		ytClient:   youtube.New(httpClient),
		store:      store.New(),
		httpSem:    make(chan struct{}, cfg.HTTPConcurrency),
		ytCfg: youtube.PipelineConfig{
			ExtractTranscripts: cfg.YouTube.ExtractTranscripts,
			TranscriptLangs:    cfg.YouTube.TranscriptLangs,
			DownloadAudio:      cfg.YouTube.DownloadAudio,
			AudioOutputDir:     cfg.YouTube.AudioOutputDir,
			SubtitleFormats:    cfg.YouTube.SubtitleFormats,
			SubtitleOutputDir:  cfg.YouTube.SubtitleOutputDir,
			TranscribeAudio:    cfg.YouTube.TranscribeAudio,
			YTDLOptions: ytdl.Options{
				CookiesFromBrowser: cfg.YouTube.CookiesFromBrowser,
				CookiesFile:        cfg.YouTube.CookiesFile,
				SleepInterval:      cfg.YouTube.SleepInterval,
				MaxSleepInterval:   cfg.YouTube.MaxSleepInterval,
				PlayerClients:      cfg.YouTube.PlayerClients,
				POToken:            cfg.YouTube.POToken,
				JSRuntime:          cfg.YouTube.JSRuntime,
				UserAgent:          cfg.YouTube.UserAgent,
			},
			ProcessingStyles: styles,
		},
		transcriber: t,
		processor:   proc,
	}
}

func (p *Pipeline) Run(ctx context.Context, urls []string) ResultStore {
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			p.processURL(ctx, url)
		}(u)
	}
	wg.Wait()
	return p.store
}

// ProcessSingle processes a single URL and streams LLM progress via progressCh.
// numCtx overrides the LLM context window size for this request; 0 uses the configured default.
// styles overrides the processing styles for this request; nil uses the configured default.
func (p *Pipeline) ProcessSingle(ctx context.Context, url string, progressCh chan<- llm.ProgressEvent, numCtx int, styles []llm.ProcessingStyle) (*store.Result, error) {
	cfg := p.ytCfg
	cfg.ProgressCh = progressCh
	cfg.NumCtx = numCtx
	if len(styles) > 0 {
		cfg.ProcessingStyles = styles
	}
	return p.dispatch(ctx, url, cfg)
}

func (p *Pipeline) processURL(ctx context.Context, url string) {
	var lastErr error
	for attempt := range p.retryAttempts {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("[retry] %s attempt %d/%d, waiting %v", url, attempt+1, p.retryAttempts, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				p.storeError(url, ctx.Err())
				return
			}
		}

		result, err := p.dispatch(ctx, url, p.ytCfg)
		if err != nil {
			lastErr = err
			log.Printf("[error] %s: %v", url, err)
			continue
		}

		p.store.Put(result)
		log.Printf("[done] %s", url)
		return
	}
	p.storeError(url, lastErr)
}

// dispatch routes a URL to either the YouTube processor or the web scraper.
// It acquires the HTTP semaphore to bound overall concurrency.
func (p *Pipeline) dispatch(ctx context.Context, url string, ytCfg youtube.PipelineConfig) (*store.Result, error) {
	select {
	case p.httpSem <- struct{}{}:
		defer func() { <-p.httpSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if ytType, id, ok := router.IsYouTube(url); ok {
		log.Printf("[router] %s → youtube (%d, %s)", url, ytType, id)
		return youtube.Process(ctx, p.ytClient, ytCfg, p.transcriber, p.processor, url, ytType, id)
	}

	return p.webScraper.Fetch(ctx, url)
}

func (p *Pipeline) storeError(url string, err error) {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	p.store.Put(&store.Result{
		URL:       url,
		Error:     errMsg,
		FetchedAt: time.Now(),
	})
}

// Close cleans up resources held by the pipeline.
func (p *Pipeline) Close() {
	p.webScraper.Close()
}
