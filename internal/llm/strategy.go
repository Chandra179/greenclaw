package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"
)

// keyPointsResponse is the intermediate map-step output.
type keyPointsResponse struct {
	KeyPoints []string `json:"key_points"`
}

var keyPointsSchema = json.RawMessage(`{"type":"object","properties":{"key_points":{"type":"array","items":{"type":"string"}}},"required":["key_points"]}`)

// processRefine summarizes a transcript with a rolling-window strategy.
// Each chunk produces an intermediate summary; subsequent chunks refine it.
// This preserves narrative continuity across chunk boundaries.
// For transcripts that fit in a single chunk, it falls back to a single call.
func (o *OllamaClient) processRefine(ctx context.Context, req Request) (*Result, error) {
	numCtx := o.effectiveNumCtx(req)
	chunks := o.effectiveChunker(req).Chunk(req.Text)

	if len(chunks) == 1 {
		raw, err := o.callWithRetry(ctx, promptSingleSummary(req.Title, req.Text), ollamaSchema(StyleSummary), numCtx)
		if err != nil {
			return nil, err
		}
		s, err := extractSummary(raw)
		if err != nil {
			return nil, err
		}
		return &Result{Style: StyleSummary, Content: []string{s}}, nil
	}

	// First chunk: build initial summary.
	emitProgress(req.ProgressCh, ProgressEvent{Type: "chunk_start", Chunk: 1, Total: len(chunks), Style: StyleSummary})
	raw, err := o.callWithRetry(ctx, promptSummaryInitial(req.Title, chunks[0]), ollamaSchema(StyleSummary), numCtx)
	if err != nil {
		return nil, fmt.Errorf("refine chunk 1: %w", err)
	}
	emitProgress(req.ProgressCh, ProgressEvent{Type: "chunk_done", Chunk: 1, Total: len(chunks), Style: StyleSummary})
	running, err := extractSummary(raw)
	if err != nil {
		return nil, fmt.Errorf("refine chunk 1: %w", err)
	}

	// Subsequent chunks: refine the running summary.
	// Cap the running summary to preserve chunk budget on small-context models.
	for i, chunk := range chunks[1:] {
		chunkNum := i + 2
		emitProgress(req.ProgressCh, ProgressEvent{Type: "chunk_start", Chunk: chunkNum, Total: len(chunks), Style: StyleSummary})
		raw, err = o.callWithRetry(ctx, promptSummaryRefine(req.Title, capSummary(running, 150), chunk), ollamaSchema(StyleSummary), numCtx)
		if err != nil {
			return nil, fmt.Errorf("refine chunk %d: %w", chunkNum, err)
		}
		emitProgress(req.ProgressCh, ProgressEvent{Type: "chunk_done", Chunk: chunkNum, Total: len(chunks), Style: StyleSummary})
		running, err = extractSummary(raw)
		if err != nil {
			return nil, fmt.Errorf("refine chunk %d: %w", chunkNum, err)
		}
	}

	return &Result{Style: StyleSummary, Content: []string{running}}, nil
}

// processMapReduce extracts takeaways with a map-reduce strategy.
// Each chunk is independently mapped to key points in parallel, then all points are
// reduced into a final deduplicated takeaways list.
// For transcripts that fit in a single chunk, it falls back to a single call.
func (o *OllamaClient) processMapReduce(ctx context.Context, req Request) (*Result, error) {
	numCtx := o.effectiveNumCtx(req)
	chunks := o.effectiveChunker(req).Chunk(req.Text)

	if len(chunks) == 1 {
		raw, err := o.callWithRetry(ctx, promptSingleTakeaways(req.Title, req.Text), ollamaSchema(StyleTakeaways), numCtx)
		if err != nil {
			return nil, err
		}
		pts, err := extractTakeaways(raw)
		if err != nil {
			return nil, err
		}
		return &Result{Style: StyleTakeaways, Content: pts}, nil
	}

	// Map: extract key points from each chunk concurrently.
	pointsPerChunk := make([][]string, len(chunks))
	g, gctx := errgroup.WithContext(ctx)
	for i, chunk := range chunks {
		g.Go(func() error {
			emitProgress(req.ProgressCh, ProgressEvent{Type: "chunk_start", Chunk: i + 1, Total: len(chunks), Style: StyleTakeaways})
			raw, err := o.callWithRetry(gctx, promptTakeawaysMap(req.Title, chunk), keyPointsSchema, numCtx)
			if err != nil {
				return fmt.Errorf("map chunk %d: %w", i+1, err)
			}
			points, err := extractKeyPoints(raw)
			if err != nil {
				return fmt.Errorf("map chunk %d: %w", i+1, err)
			}
			pointsPerChunk[i] = points
			emitProgress(req.ProgressCh, ProgressEvent{Type: "chunk_done", Chunk: i + 1, Total: len(chunks), Style: StyleTakeaways})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var allPoints []string
	for _, pts := range pointsPerChunk {
		allPoints = append(allPoints, pts...)
	}

	// Reduce: consolidate all points into final takeaways.
	emitProgress(req.ProgressCh, ProgressEvent{Type: "reduce_start", Chunk: 0, Total: len(chunks), Style: StyleTakeaways})
	raw, err := o.callWithRetry(ctx, promptTakeawaysReduce(req.Title, allPoints), ollamaSchema(StyleTakeaways), numCtx)
	if err != nil {
		return nil, fmt.Errorf("reduce: %w", err)
	}
	emitProgress(req.ProgressCh, ProgressEvent{Type: "reduce_done", Chunk: 0, Total: len(chunks), Style: StyleTakeaways})
	pts, err := extractTakeaways(raw)
	if err != nil {
		return nil, fmt.Errorf("reduce: %w", err)
	}
	return &Result{Style: StyleTakeaways, Content: pts}, nil
}

// capSummary truncates s to at most maxWords words, appending "..." if truncated.
// This keeps the running summary from consuming too much of a small model's context window.
func capSummary(s string, maxWords int) string {
	words := strings.Fields(s)
	if len(words) <= maxWords {
		return s
	}
	return strings.Join(words[:maxWords], " ") + "..."
}

// --- Prompt builders ---

func promptSingleSummary(title, text string) string {
	return fmt.Sprintf(`Summarize the following text in two concise paragraphs.

Video title: %s

Transcript:
%s

Respond with JSON in this exact format:
{"summary": "your summary here"}`, title, text)
}

func promptSummaryInitial(title, chunk string) string {
	return fmt.Sprintf(`Summarize this text, titled "%s".

Transcript:
%s

Respond with JSON: {"summary": "your summary here"}`, title, chunk)
}

func promptSummaryRefine(title, runningSummary, chunk string) string {
	return fmt.Sprintf(`You are building a running summary of a YouTube video titled "%s".

Current summary:
%s

Next transcript portion:
%s

Update the summary to incorporate the new content. Be concise.

Respond with JSON: {"summary": "updated summary here"}`, title, runningSummary, chunk)
}

func promptSingleTakeaways(title, text string) string {
	return fmt.Sprintf(`Extract the key takeaways from the following YouTube video transcript. Return up to 10 bullet points ordered by importance.

Video title: %s

Transcript:
%s

Respond with JSON in this exact format:
{"takeaways": [{"text": "takeaway 1"}, {"text": "takeaway 2"}]}`, title, text)
}

func promptTakeawaysMap(title, chunk string) string {
	return fmt.Sprintf(`Extract up to 5 key points from this portion of a YouTube video transcript titled "%s".

Transcript:
%s

Respond with JSON: {"key_points": ["point 1", "point 2"]}`, title, chunk)
}

func promptTakeawaysReduce(title string, points []string) string {
	var sb strings.Builder
	for _, p := range points {
		sb.WriteString("- ")
		sb.WriteString(p)
		sb.WriteByte('\n')
	}
	return fmt.Sprintf(`Below are key points extracted from different portions of a YouTube video titled "%s".

%s
Consolidate into up to 10 final takeaways ordered by importance. Remove duplicates.

Respond with JSON in this exact format:
{"takeaways": [{"text": "takeaway 1"}, {"text": "takeaway 2"}]}`, title, sb.String())
}

// --- Extraction helpers ---

func extractSummary(raw json.RawMessage) (string, error) {
	var s SummaryResponse
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("parse summary response: %w", err)
	}
	if s.Summary == "" {
		return "", fmt.Errorf("model returned empty summary")
	}
	return s.Summary, nil
}

func extractKeyPoints(raw json.RawMessage) ([]string, error) {
	var k keyPointsResponse
	if err := json.Unmarshal(raw, &k); err != nil {
		return nil, fmt.Errorf("parse key points response: %w", err)
	}
	return k.KeyPoints, nil
}

func extractTakeaways(raw json.RawMessage) ([]string, error) {
	var resp TakeawaysResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse takeaways response: %w", err)
	}
	out := make([]string, len(resp.Takeaways))
	for i, t := range resp.Takeaways {
		out[i] = t.Text
	}
	return out, nil
}
