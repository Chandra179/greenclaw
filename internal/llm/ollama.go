package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const maxAttempts = 2

// OllamaClient talks to an Ollama instance for LLM processing.
type OllamaClient struct {
	endpoint    string
	model       string
	numCtx      int
	overlapChars int
	chunker     Chunker
	client      *http.Client
	cache       *ResultCache
}

// NewOllamaClient creates a client for the given Ollama endpoint and model.
// overlapTokens controls how many tokens of context are repeated across chunk
// boundaries; pass 0 to use the default (200 tokens).
// cacheDir is the directory for the file-based result cache; pass "" to disable.
func NewOllamaClient(endpoint, model string, timeout time.Duration, numCtx, overlapTokens int, cacheDir string) *OllamaClient {
	const (
		charsPerToken  = 4
		reservedTokens = 1000
		defaultOverlap = 200
	)
	chunkSize := (numCtx - reservedTokens) * charsPerToken
	if chunkSize <= 0 {
		chunkSize = charsPerToken * 100
	}
	overlap := overlapTokens
	if overlap <= 0 {
		overlap = defaultOverlap
	}

	var cache *ResultCache
	if cacheDir != "" {
		var err error
		cache, err = NewResultCache(cacheDir)
		if err != nil {
			log.Printf("[llm] cache unavailable (%s): %v", cacheDir, err)
		}
	}

	return &OllamaClient{
		endpoint:     strings.TrimRight(endpoint, "/"),
		model:        model,
		numCtx:       numCtx,
		overlapChars: overlap * charsPerToken,
		chunker:      RecursiveChunker{ChunkSize: chunkSize, Overlap: overlap * charsPerToken},
		client:       &http.Client{Timeout: timeout},
		cache:        cache,
	}
}

// effectiveNumCtx returns req.NumCtx if set, otherwise the client default.
func (o *OllamaClient) effectiveNumCtx(req Request) int {
	if req.NumCtx > 0 {
		return req.NumCtx
	}
	return o.numCtx
}

// effectiveChunker returns a chunker sized for req.NumCtx, or the default chunker.
func (o *OllamaClient) effectiveChunker(req Request) Chunker {
	if req.NumCtx <= 0 || req.NumCtx == o.numCtx {
		return o.chunker
	}
	const (charsPerToken = 4; reservedTokens = 1000)
	chunkSize := (req.NumCtx - reservedTokens) * charsPerToken
	if chunkSize <= 0 {
		chunkSize = charsPerToken * 100
	}
	return RecursiveChunker{ChunkSize: chunkSize, Overlap: o.overlapChars}
}

// Process implements Client. It checks the file cache first, then dispatches to a
// style-appropriate multi-step strategy. Results are stored in the cache on success.
func (o *OllamaClient) Process(ctx context.Context, req Request) (*Result, error) {
	numCtx := o.effectiveNumCtx(req)
	if o.cache != nil && req.CacheKey != "" {
		if r, ok := o.cache.Get(req.CacheKey, string(req.Style), o.model, numCtx); ok {
			log.Printf("[llm] cache hit %s/%s", req.CacheKey, req.Style)
			return r, nil
		}
	}

	var (
		result *Result
		err    error
	)
	switch req.Style {
	case StyleSummary:
		result, err = o.processRefine(ctx, req)
	case StyleTakeaways:
		result, err = o.processMapReduce(ctx, req)
	default:
		var raw json.RawMessage
		raw, err = o.callWithRetry(ctx, buildDefaultPrompt(req), nil, numCtx)
		if err == nil {
			result = &Result{Style: req.Style, Content: raw}
		}
	}

	if err != nil {
		return nil, err
	}

	if o.cache != nil && req.CacheKey != "" {
		if writeErr := o.cache.Put(req.CacheKey, string(req.Style), o.model, numCtx, result); writeErr != nil {
			log.Printf("[llm] cache write failed: %v", writeErr)
		}
	}

	return result, nil
}

// callWithRetry calls attempt up to maxAttempts times, logging transient errors.
func (o *OllamaClient) callWithRetry(ctx context.Context, prompt string, format json.RawMessage, numCtx int) (json.RawMessage, error) {
	if format == nil {
		format = json.RawMessage(`"json"`)
	}
	var lastErr error
	for attempt := range maxAttempts {
		raw, err := o.attempt(ctx, prompt, format, numCtx)
		if err == nil {
			return raw, nil
		}
		lastErr = err
		if attempt < maxAttempts-1 {
			log.Printf("[llm] attempt %d failed, retrying: %v", attempt+1, err)
		}
	}
	return nil, lastErr
}

// attempt makes a single HTTP request to Ollama and returns the raw JSON response text.
func (o *OllamaClient) attempt(ctx context.Context, prompt string, format json.RawMessage, numCtx int) (json.RawMessage, error) {
	body, err := json.Marshal(ollamaRequest{
		Model:   o.model,
		Prompt:  prompt,
		Format:  format,
		Stream:  false,
		Options: ollamaOptions{NumCtx: numCtx},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, b)
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	return json.RawMessage(ollamaResp.Response), nil
}

func buildDefaultPrompt(req Request) string {
	return fmt.Sprintf(`Process the following transcript.

Video title: %s

Transcript:
%s

Respond with valid JSON.`, req.Title, req.Text)
}

type ollamaOptions struct {
	NumCtx int `json:"num_ctx"`
}

type ollamaRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Format  json.RawMessage `json:"format"`
	Stream  bool            `json:"stream"`
	Options ollamaOptions   `json:"options"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}
