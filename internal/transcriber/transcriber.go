package transcriber

import "context"

// Options configures a transcription run.
type Options struct {
	Model    string // Model size: tiny, base, small, medium, large-v3
	ModelDir string // Directory containing model files
	Language string // ISO 639-1 code, empty for auto-detect
	Task     string // "transcribe" or "translate" (to English)
}

// Result holds the output of a transcription.
type Result struct {
	Text     string  // Transcribed text
	Language string  // Detected or specified language
	Duration float64 // Audio duration in seconds
}

// Transcriber transcribes audio files to text.
type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string, opts Options) (*Result, error)
}
