package llm

// ProgressEvent is emitted during long LLM processing to report chunk-level progress.
type ProgressEvent struct {
	Type  string          `json:"type"`  // "chunk_start", "chunk_done", "reduce_start", "reduce_done"
	Chunk int             `json:"chunk"` // 1-based chunk index; 0 for the reduce step
	Total int             `json:"total"` // total number of chunks in the map phase
	Style ProcessingStyle `json:"style"`
}

// emitProgress sends ev to ch without blocking. Progress events are best-effort:
// if the consumer is slow, the event is silently dropped.
func emitProgress(ch chan<- ProgressEvent, ev ProgressEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- ev:
	default:
	}
}
