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

// OllamaClient talks to an Ollama instance for LLM processing.
type OllamaClient struct {
	endpoint string
	model    string
	numCtx   int
	client   *http.Client
}

// NewOllamaClient creates a client for the given Ollama endpoint and model.
func NewOllamaClient(endpoint, model string, timeout time.Duration, numCtx int) *OllamaClient {
	return &OllamaClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		numCtx:   numCtx,
		client:   &http.Client{Timeout: timeout},
	}
}

func (o *OllamaClient) Process(ctx context.Context, req Request) (*Result, error) {
	prompt := buildPrompt(req)

	body, err := json.Marshal(ollamaRequest{
		Model:   o.model,
		Prompt:  prompt,
		Format:  "json",
		Stream:  false,
		Options: ollamaOptions{NumCtx: o.numCtx},
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

	// Validate the response is valid JSON
	raw := json.RawMessage(ollamaResp.Response)
	if !json.Valid(raw) {
		return nil, fmt.Errorf("ollama returned invalid JSON: %.200s", ollamaResp.Response)
	}

	return &Result{
		Style:   req.Style,
		Content: raw,
	}, nil
}

type ollamaOptions struct {
	NumCtx int `json:"num_ctx"`
}

type ollamaRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	Format  string        `json:"format"`
	Stream  bool          `json:"stream"`
	Options ollamaOptions `json:"options"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func buildPrompt(req Request) string {
	switch req.Style {
	case StyleSummary:
		return fmt.Sprintf(`Summarize the following YouTube video transcript in one or two concise paragraphs.

Video title: %s
Duration: %s

Transcript:
%s

Respond with JSON in this exact format:
{"summary": "your summary here"}`, req.Title, req.Duration, req.Text)

	case StyleTakeaways:
		return fmt.Sprintf(`Extract the key takeaways from the following YouTube video transcript. Return up to 10 bullet points ordered by importance.

Video title: %s
Duration: %s

Transcript:
%s

Respond with JSON in this exact format:
{"takeaways": [{"text": "takeaway 1"}, {"text": "takeaway 2"}]}`, req.Title, req.Duration, req.Text)

	default:
		return fmt.Sprintf(`Process the following transcript.

Video title: %s
Duration: %s

Transcript:
%s

Respond with valid JSON.`, req.Title, req.Duration, req.Text)
	}
}
