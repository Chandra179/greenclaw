package llm

import (
	"context"
	"encoding/json"
)

// ProcessingStyle is a named post-processing mode.
type ProcessingStyle string

const (
	StyleSummary   ProcessingStyle = "summary"
	StyleTakeaways ProcessingStyle = "takeaways"
)

// Request bundles everything the processor needs for any style.
type Request struct {
	Style    ProcessingStyle
	Title    string
	Duration string // "MM:SS"
	Text     string // plain-text transcript
}

// Result is the style-agnostic envelope returned by Process.
type Result struct {
	Style   ProcessingStyle `json:"style"`
	Content json.RawMessage `json:"content"`
}

// Client is the interface all LLM backends implement.
type Client interface {
	Process(ctx context.Context, req Request) (*Result, error)
}
