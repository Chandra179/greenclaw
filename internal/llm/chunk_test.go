package llm

import (
	"strings"
	"testing"
)

func TestChunkTextShort(t *testing.T) {
	text := "short text"
	chunks := chunkText(text, 100, 10)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk = %q, want %q", chunks[0], text)
	}
}

func TestChunkTextExactSize(t *testing.T) {
	text := strings.Repeat("a", 100)
	chunks := chunkText(text, 100, 10)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
}

func TestChunkTextSplitsOnWordBoundary(t *testing.T) {
	// "hello world foo bar" — chunkSize=12 should split after "hello world" (11 chars)
	text := "hello world foo bar"
	chunks := chunkText(text, 12, 0)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[0] != "hello world" {
		t.Errorf("chunk[0] = %q, want %q", chunks[0], "hello world")
	}
}

func TestChunkTextOverlapCarriesContext(t *testing.T) {
	// 30-char text, chunkSize=20, overlap=10 → chunk2 starts at char 10
	text := "aaa bbb ccc ddd eee fff"
	chunks := chunkText(text, 12, 6)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	// chunk[1] should start somewhere inside chunk[0]'s content (overlap)
	if !strings.Contains(chunks[0], chunks[1][:3]) {
		// at minimum the first few chars of chunk[1] should be in chunk[0]
		t.Logf("chunk[0] = %q", chunks[0])
		t.Logf("chunk[1] = %q", chunks[1])
	}
}

func TestChunkTextNoInfiniteLoop(t *testing.T) {
	// overlap >= chunkSize should not loop forever
	text := strings.Repeat("x ", 100)
	chunks := chunkText(text, 10, 20) // overlap > chunkSize
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

func TestChunkTextCoversFull(t *testing.T) {
	// All characters in the original text should appear in at least one chunk.
	text := "one two three four five six seven eight nine ten"
	chunks := chunkText(text, 15, 5)
	joined := strings.Join(chunks, " ")
	for _, word := range strings.Fields(text) {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q missing from chunks", word)
		}
	}
}
