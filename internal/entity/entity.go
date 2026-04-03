package entity

import "context"

// EntityType classifies the kind of entity.
type EntityType string

const (
	EntityTypeTopic   EntityType = "topic"
	EntityTypeConcept EntityType = "concept"
)

// Entity is a named node extracted from video content.
type Entity struct {
	// Key is the canonical, normalised identifier used as the graph vertex key.
	// Derived from Name: lowercased, whitespace collapsed, non-alphanumeric stripped.
	Key  string
	Name string
	Type EntityType
}

// ExtractionRequest bundles the content fields the extractor needs.
type ExtractionRequest struct {
	VideoURL    string
	VideoID     string
	Title       string
	Description string
	// ContentText is the concatenated takeaway/summary text from LLM processing results.
	ContentText string
}

// Extractor extracts named entities from video content.
type Extractor interface {
	Extract(ctx context.Context, req ExtractionRequest) ([]Entity, error)
}
