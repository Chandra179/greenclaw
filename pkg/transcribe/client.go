package transcribe

import "context"

// Result holds the output of a transcription.
type Result struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
}

// Client transcribes audio files to text.
type Client interface {
	Transcribe(ctx context.Context, audioPath string) (*Result, error)
}
