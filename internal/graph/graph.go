package graph

import "context"

// ExtractionRequest bundles the content fields the extractor needs.
type ExtractionRequest struct {
	VideoURL    string
	VideoID     string
	Title       string
	Description string
	// ContentText is the concatenated takeaway/summary text from LLM processing results.
	ContentText string
}

// EntityExtractor extracts named entities from video content.
type EntityExtractor interface {
	Extract(ctx context.Context, req ExtractionRequest) ([]Entity, error)
}
