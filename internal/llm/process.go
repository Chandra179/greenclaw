package llm

import "context"

// ProcessingStyle is a named post-processing mode.
type ProcessingStyle string

const (
	StyleSummary   ProcessingStyle = "summary"
	StyleTakeaways ProcessingStyle = "takeaways"
)

// Request bundles everything the processor needs for any style.
type Request struct {
	Style      ProcessingStyle
	Title      string
	Text       string              // plain-text transcript
	CacheKey   string              // optional; when set, enables file-based result caching
	ProgressCh chan<- ProgressEvent // optional; nil = no progress reporting
	NumCtx     int                 // optional; overrides the client's default context window size
}

// Result is the style-agnostic envelope returned by Process.
// Content is always a []string: one element for summary, multiple for takeaways.
type Result struct {
	Style   ProcessingStyle `json:"style"`
	Content []string        `json:"content"`
}

// Client is the interface all LLM backends implement.
type Client interface {
	Process(ctx context.Context, req Request) (*Result, error)
}
