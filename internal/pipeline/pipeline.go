package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"greenclaw/internal/browser"
	"greenclaw/internal/config"
	"greenclaw/internal/constant"
	"greenclaw/internal/fetcher"
	"greenclaw/internal/graph"
	"greenclaw/internal/llm"
	"greenclaw/internal/store"
	"greenclaw/internal/youtube"
	"greenclaw/pkg/graphdb"
	"greenclaw/pkg/httpclient"
	"greenclaw/pkg/transcribe"
	"greenclaw/pkg/ytdl"
)

// Pipeline orchestrates URL processing, routing to either the web scraper or
// the YouTube processor based on the URL type.
type Pipeline struct {
	retryAttempts int
	client        *http.Client
	browserPool   browser.BrowserFetcher
	browserSem    chan struct{}
	ytClient      *youtube.Client
	httpSem       chan struct{}
	ytCfg         youtube.PipelineConfig
	transcriber   transcribe.Client
	processor     llm.Client
	extractor     graph.EntityExtractor
	kg            graphdb.Store
}

func NewPipeline(cfg config.Config) *Pipeline {
	client := httpclient.New(cfg.Timeout)

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
		client:        client,
		browserPool:   browser.NewPool(cfg.RecycleAfter),
		browserSem:    make(chan struct{}, cfg.BrowserConcurrency),
		ytClient:      youtube.New(client),
		httpSem:       make(chan struct{}, cfg.HTTPConcurrency),
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
		ollamaClient, ok := proc.(*llm.OllamaClient)
		if !ok {
			log.Printf("[graph] knowledge graph requires an LLM client; configure llm.processing_styles to enable it")
		} else {
			kg, err := graphdb.NewArangoGraph(context.Background(), cfg.Graph)
			if err != nil {
				log.Printf("[graph] init failed, knowledge graph disabled: %v", err)
			} else {
				p.extractor = graph.NewOllamaEntityExtractor(ollamaClient, cfg.LLM.NumCtx)
				p.kg = kg
			}
		}
	}

	return p
}

// ProcessSingle processes a single URL and streams LLM progress via progressCh.
// The result is stored so it can be retrieved later (e.g. for graph indexing).
func (p *Pipeline) ProcessSingle(ctx context.Context, url string, progressCh chan<- llm.ProgressEvent,
	numCtx int, styles []llm.ProcessingStyle) (*store.Result, error) {
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
	if p.kg != nil {
		if serr := store.SaveResult(ctx, p.kg, result); serr != nil {
			log.Printf("[graph] save result for %s: %v", url, serr)
		}
	}
	return result, nil
}

// dispatch routes a URL to either the YouTube processor or the web scraper.
func (p *Pipeline) dispatch(ctx context.Context, url string, ytCfg youtube.PipelineConfig) (*store.Result, error) {
	select {
	case p.httpSem <- struct{}{}:
		defer func() { <-p.httpSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if ytType, id, ok := youtube.Detect(url); ok {
		log.Printf("[router] %s → youtube (%d, %s)", url, ytType, id)
		return youtube.Process(ctx, p.ytClient, ytCfg, p.transcriber, p.processor, url, ytType, id)
	}

	return p.fetchWeb(ctx, url)
}

// fetchWeb classifies and fetches a web URL, escalating to browser if needed.
func (p *Pipeline) fetchWeb(ctx context.Context, url string) (*store.Result, error) {
	ct, err := p.classify(ctx, url)
	if err != nil {
		log.Printf("[router] HEAD failed for %s, assuming HTML: %v", url, err)
		ct = constant.HTTPContentHTML
	}

	log.Printf("[router] %s → %s", url, ct)

	switch ct {
	case constant.HTTPContentBinary:
		return fetcher.DownloadBinary(ctx, p.client, url, "downloads")
	case constant.HTTPContentJSON:
		return fetcher.FetchJSON(ctx, p.client, url)
	case constant.HTTPContentXML:
		return fetcher.FetchXML(ctx, p.client, url)
	default:
		return p.fetchHTML(ctx, url)
	}
}

func (p *Pipeline) fetchHTML(ctx context.Context, url string) (*store.Result, error) {
	result, err := fetcher.FetchHTML(ctx, p.client, url)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, fetcher.ErrNeedsEscalation) {
		return nil, err
	}

	log.Printf("[escalate] %s → browser", url)

	select {
	case p.browserSem <- struct{}{}:
		defer func() { <-p.browserSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return p.browserPool.FetchPage(ctx, url)
}

func (p *Pipeline) IndexResult(ctx context.Context, url string) {
	if p.extractor == nil || p.kg == nil {
		return
	}

	result, err := store.GetResult(ctx, p.kg, url)
	if err != nil {
		log.Printf("[graph] get result for %s: %v", url, err)
		return
	}

	req := graph.ExtractionRequest{
		VideoURL:    result.URL,
		Title:       result.Title,
		Description: result.Description,
		ContentText: result.Text,
	}
	if result.YouTube != nil {
		req.VideoID = result.YouTube.VideoID
	}

	entities, err := p.extractor.Extract(ctx, req)
	if err != nil {
		log.Printf("[graph] extract entities for %s: %v", url, err)
		return
	}
	if len(entities) == 0 {
		return
	}

	nodes := make([]store.EntityNode, len(entities))
	keys := make([]string, len(entities))
	for i, e := range entities {
		nodes[i] = store.EntityNode{Key: e.Key, Name: e.Name, Type: store.EntityType(e.Type)}
		keys[i] = e.Key
	}

	videoNode := store.VideoNode{
		Key:         url,
		URL:         result.URL,
		Title:       result.Title,
		Description: result.Description,
	}
	if result.YouTube != nil {
		videoNode.Key = result.YouTube.VideoID
	}

	if err := store.UpsertVideo(ctx, p.kg, videoNode); err != nil {
		log.Printf("[graph] upsert video %s: %v", videoNode.Key, err)
		return
	}
	if err := store.UpsertEntities(ctx, p.kg, nodes); err != nil {
		log.Printf("[graph] upsert entities for %s: %v", videoNode.Key, err)
		return
	}
	if err := store.AddMentions(ctx, p.kg, videoNode.Key, keys); err != nil {
		log.Printf("[graph] add mentions for %s: %v", videoNode.Key, err)
		return
	}
	if err := store.AddRelated(ctx, p.kg, keys); err != nil {
		log.Printf("[graph] add related for %s: %v", videoNode.Key, err)
	}
}

// Close cleans up resources held by the pipeline.
func (p *Pipeline) Close() {
	p.browserPool.Close()
	if p.kg != nil {
		if err := p.kg.Close(); err != nil {
			log.Printf("[graph] close: %v", err)
		}
	}
}

// Classify performs a HEAD request and returns the content type classification.
func (p *Pipeline) classify(ctx context.Context, url string) (constant.HTTPContentType, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating HEAD request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HEAD request: %w", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		// Fall back to HTML if no content type header
		return constant.HTTPContentHTML, nil
	}

	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		// If we can't parse, try raw string matching
		mediaType = strings.ToLower(strings.TrimSpace(ct))
	}

	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		return constant.HTTPContentHTML, nil
	case mediaType == "application/json":
		return constant.HTTPContentJSON, nil
	case mediaType == "text/xml" || mediaType == "application/xml" ||
		strings.HasSuffix(mediaType, "+xml"):
		return constant.HTTPContentXML, nil
	case strings.HasPrefix(mediaType, "image/") ||
		mediaType == "application/pdf" ||
		mediaType == "application/octet-stream":
		return constant.HTTPContentBinary, nil
	default:
		// Default to HTML for unknown text types, binary for others
		if strings.HasPrefix(mediaType, "text/") {
			return constant.HTTPContentHTML, nil
		}
		return constant.HTTPContentBinary, nil
	}
}
