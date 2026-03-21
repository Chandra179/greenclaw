package transcriber

import (
	"context"
	"fmt"
	"greenclaw/internal/config"
	"time"
)

// Result holds the output of a transcription.
type Result struct {
	Text     string  // Transcribed text
	Language string  // Detected or specified language
	Duration float64 // Audio duration in seconds
}

// Client transcribes audio files to text.
// Configuration (model, language, etc.) is baked in at construction time.
type Client interface {
	Transcribe(ctx context.Context, audioPath string) (*Result, error)
}

// New creates an HTTPTranscriber from the configuration.
func New(cfg config.TranscriberConfig) (Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("transcriber endpoint is required")
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = 5 * time.Minute
	}
	return NewHTTPTranscriber(cfg.Endpoint, timeout, cfg.Language), nil
}
