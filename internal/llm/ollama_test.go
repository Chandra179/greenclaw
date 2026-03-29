package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*OllamaClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewOllamaClient(srv.URL, "test-model", 10*time.Second, 4096, 200, ""), srv
}

func ollamaHandler(response string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaResponse{Response: response})
	}
}

// sequentialHandler returns a handler that serves responses[i] on the i-th call.
// After exhausting the list it repeats the last entry.
func sequentialHandler(responses []string) (http.HandlerFunc, *atomic.Int32) {
	var calls atomic.Int32
	return func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		if i >= len(responses) {
			i = len(responses) - 1
		}
		json.NewEncoder(w).Encode(ollamaResponse{Response: responses[i]})
	}, &calls
}

// TestProcessSummary verifies that a short transcript produces a single summary call.
func TestProcessSummary(t *testing.T) {
	payload := `{"summary": "This is a valid summary of the video content."}`
	client, _ := newTestClient(t, ollamaHandler(payload))

	result, err := client.Process(context.Background(), Request{
		Style: StyleSummary,
		Title: "Test Video",
		Text:  "some transcript text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Style != StyleSummary {
		t.Errorf("style = %q, want %q", result.Style, StyleSummary)
	}

	if len(result.Content) != 1 || result.Content[0] == "" {
		t.Errorf("expected one non-empty summary, got %v", result.Content)
	}
}

// TestProcessTakeaways verifies that a short transcript produces a single takeaways call.
func TestProcessTakeaways(t *testing.T) {
	payload := `{"takeaways": [{"text": "First key point"}, {"text": "Second key point"}]}`
	client, _ := newTestClient(t, ollamaHandler(payload))

	result, err := client.Process(context.Background(), Request{
		Style: StyleTakeaways,
		Title: "Test Video",
		Text:  "some transcript text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) != 2 {
		t.Errorf("takeaways count = %d, want 2", len(result.Content))
	}
}

// TestProcessSummaryMultiChunk verifies rolling-window refine for long transcripts.
// Two chunks → 2 LLM calls; final result carries the last refined summary.
func TestProcessSummaryMultiChunk(t *testing.T) {
	// numCtx=4096 → chunkSize=(4096-1000)*4=12384 chars.
	// A 14000-char transcript produces exactly 2 chunks.
	text := strings.Repeat("word ", 2800) // 14000 chars

	handler, calls := sequentialHandler([]string{
		`{"summary": "initial summary"}`,
		`{"summary": "refined summary"}`,
	})
	client, _ := newTestClient(t, handler)

	result, err := client.Process(context.Background(), Request{
		Style: StyleSummary,
		Title: "Long Video",
		Text:  text,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("LLM calls = %d, want 2", calls.Load())
	}

	if len(result.Content) != 1 || result.Content[0] != "refined summary" {
		t.Errorf("content = %v, want [\"refined summary\"]", result.Content)
	}
}

// TestProcessTakeawaysMultiChunk verifies map-reduce for long transcripts.
// Two chunks → 2 map calls + 1 reduce call = 3 LLM calls total.
func TestProcessTakeawaysMultiChunk(t *testing.T) {
	text := strings.Repeat("word ", 2800) // 14000 chars → 2 chunks

	handler, calls := sequentialHandler([]string{
		`{"key_points": ["point A", "point B"]}`, // map chunk 1
		`{"key_points": ["point C"]}`,             // map chunk 2
		`{"takeaways": [{"text": "final point"}]}`, // reduce
	})
	client, _ := newTestClient(t, handler)

	result, err := client.Process(context.Background(), Request{
		Style: StyleTakeaways,
		Title: "Long Video",
		Text:  text,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("LLM calls = %d, want 3", calls.Load())
	}

	if len(result.Content) != 1 || result.Content[0] != "final point" {
		t.Errorf("content = %v, want [\"final point\"]", result.Content)
	}
}

func TestProcessRetriesOnServerError(t *testing.T) {
	attempts := 0
	payload := `{"summary": "Recovered after transient error."}`

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(ollamaResponse{Response: payload})
	})

	result, err := client.Process(context.Background(), Request{
		Style: StyleSummary,
		Title: "Test Video",
		Text:  "some transcript text",
	})
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestProcessFailsAfterMaxAttempts(t *testing.T) {
	attempts := 0
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "always failing", http.StatusInternalServerError)
	})

	_, err := client.Process(context.Background(), Request{
		Style: StyleSummary,
		Title: "Test Video",
		Text:  "some transcript text",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != maxAttempts {
		t.Errorf("attempts = %d, want %d", attempts, maxAttempts)
	}
}

func TestOllamaSchemaFormat(t *testing.T) {
	var capturedBody ollamaRequest

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(ollamaResponse{Response: `{"summary": "test summary value"}`})
	})

	client.Process(context.Background(), Request{
		Style: StyleSummary,
		Title: "Test",
		Text:  "transcript",
	})

	// Format should be a JSON object (schema), not the string "json"
	var formatStr string
	if err := json.Unmarshal(capturedBody.Format, &formatStr); err == nil {
		t.Errorf("expected schema object in format field, got string: %q", formatStr)
	}

	var formatObj map[string]any
	if err := json.Unmarshal(capturedBody.Format, &formatObj); err != nil {
		t.Errorf("format field is not a valid JSON object: %v", err)
	}
}
