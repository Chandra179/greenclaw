package pipeline

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"greenclaw/internal/browser"
	"greenclaw/internal/config"
	"greenclaw/internal/entity"
	"greenclaw/internal/graph"
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
	extractor     entity.Extractor
	kg            graph.KnowledgeGraph
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

	p := &Pipeline{
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

	if cfg.Graph.Enabled {
		var ollamaClient *llm.OllamaClient
		if oc, ok := proc.(*llm.OllamaClient); ok {
			ollamaClient = oc
		} else {
			d, err := time.ParseDuration(cfg.LLM.Timeout)
			if err != nil {
				d = 60 * time.Second
			}
			ollamaClient = llm.NewOllamaClient(cfg.LLM.Endpoint, cfg.LLM.Model, d, cfg.LLM.NumCtx, cfg.LLM.OverlapTokens, "")
		}
		kg, err := graph.NewArangoGraph(context.Background(), cfg.Graph)
		if err != nil {
			log.Printf("[graph] init failed, knowledge graph disabled: %v", err)
		} else {
			p.extractor = entity.NewOllamaExtractor(ollamaClient, cfg.LLM.NumCtx)
			p.kg = kg
		}
	}

	return p
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
	result, err := p.dispatch(ctx, url, cfg)
	if err != nil {
		return nil, err
	}
	p.indexResult(ctx, result)
	return result, nil
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
		p.indexResult(ctx, result)
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

// indexResult runs entity extraction and stores results in the knowledge graph.
// All errors are logged and swallowed — graph indexing is non-fatal.
func (p *Pipeline) indexResult(ctx context.Context, result *store.Result) {
	if p.extractor == nil || p.kg == nil {
		return
	}
	if result.YouTube == nil || result.YouTube.VideoID == "" {
		return
	}

	var sb strings.Builder
	for _, pr := range result.YouTube.Processing {
		for _, line := range pr.Content {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}

	req := entity.ExtractionRequest{
		VideoURL:    result.URL,
		VideoID:     result.YouTube.VideoID,
		Title:       result.Title,
		Description: result.Description,
		ContentText: sb.String(),
	}

	entities, err := p.extractor.Extract(ctx, req)
	if err != nil {
		log.Printf("[graph] entity extraction failed for %s: %v", result.YouTube.VideoID, err)
		return
	}
	if len(entities) == 0 {
		return
	}

	nodes := make([]graph.EntityNode, len(entities))
	keys := make([]string, len(entities))
	for i, e := range entities {
		nodes[i] = graph.EntityNode{Key: e.Key, Name: e.Name, Type: e.Type}
		keys[i] = e.Key
	}

	videoKey := result.YouTube.VideoID
	if err := p.kg.UpsertVideo(ctx, graph.VideoNode{
		Key:         videoKey,
		URL:         result.URL,
		Title:       result.Title,
		Description: result.Description,
	}); err != nil {
		log.Printf("[graph] upsert video %s: %v", videoKey, err)
		return
	}
	if err := p.kg.UpsertEntities(ctx, nodes); err != nil {
		log.Printf("[graph] upsert entities for %s: %v", videoKey, err)
		return
	}
	if err := p.kg.AddMentions(ctx, videoKey, keys); err != nil {
		log.Printf("[graph] add mentions for %s: %v", videoKey, err)
		return
	}
	if err := p.kg.AddRelated(ctx, keys); err != nil {
		log.Printf("[graph] add related for %s: %v", videoKey, err)
	}
}

// Close cleans up resources held by the pipeline.
func (p *Pipeline) Close() {
	p.webScraper.Close()
	if p.kg != nil {
		if err := p.kg.Close(); err != nil {
			log.Printf("[graph] close: %v", err)
		}
	}
}
