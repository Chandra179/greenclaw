package llm

import "strings"

type Chunker interface {
	Chunk(text string) []string
}

type RecursiveChunker struct {
	ChunkSize int
	Overlap   int
}

func (c RecursiveChunker) Chunk(text string) []string {
	return chunkText(text, c.ChunkSize, c.Overlap)
}

// splitSeps is tried in order when finding a break point within a chunk.
// Mirrors LangChain's RecursiveCharacterTextSplitter: prefer larger semantic
// units (paragraphs, newlines, sentence endings) before falling back to words.
var splitSeps = []string{"\n\n", "\n", ". ", "! ", "? ", " "}

// chunkText splits text into overlapping chunks of at most chunkSize bytes,
// breaking at the largest available semantic boundary (paragraph → sentence →
// word). overlap bytes from the end of each chunk are repeated at the start of
// the next to preserve context across boundaries.
func chunkText(text string, chunkSize, overlap int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}
	var chunks []string
	start := 0
	for start < len(text) {
		end := start + chunkSize
		if end >= len(text) {
			chunks = append(chunks, strings.TrimSpace(text[start:]))
			break
		}
		// Try separators from largest semantic unit to smallest.
		cut := end
		for _, sep := range splitSeps {
			if i := strings.LastIndex(text[start:end], sep); i > 0 {
				cut = start + i + len(sep) // keep sep with left chunk; TrimSpace cleans whitespace
				break
			}
		}
		chunks = append(chunks, strings.TrimSpace(text[start:cut]))
		next := cut - overlap
		if next <= start {
			next = cut // safety: avoid infinite loop if overlap >= chunkSize
		}
		start = next
	}
	return chunks
}
