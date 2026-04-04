package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxAttempts = 2

type OllamaClient struct {
	endpoint     string
	model        string
	numCtx       int
	overlapChars int
	client       *http.Client
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

type ollamaOptions struct {
	NumCtx int `json:"num_ctx"`
}

func NewOllamaClient(endpoint, model string, timeout time.Duration,
	numCtx, overlapTokens int) *OllamaClient {
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

	return &OllamaClient{
		endpoint:     strings.TrimRight(endpoint, "/"),
		model:        model,
		numCtx:       numCtx,
		overlapChars: overlap * charsPerToken,
		client:       &http.Client{Timeout: timeout},
	}
}

func (o *OllamaClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(ollamaRequest{
		Model:   o.model,
		Prompt:  req.Prompt,
		Format:  req.Schema,
		Stream:  false,
		Options: ollamaOptions{NumCtx: req.NumCtx},
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

	return &ChatResponse{JsonResponse: json.RawMessage(ollamaResp.Response)}, nil
}
