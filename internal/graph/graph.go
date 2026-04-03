package graph

import "context"

// ExtractionRequest bundles the content fields the extractor needs.
type ExtractionRequest struct {
	VideoURL    string
	VideoID     string
	Title       string
	Description string
	// ContentText is the concatenated takeaway/summary text from LLM processing.
	ContentText string

	// Category is the explicit domain tag assigned by the caller (e.g. from a
	// playlist label or user-supplied metadata). Takes precedence over AutoCategory.
	Category Category

	// AutoCategory is set by CategoryClassifier when Category is empty.
	// Kept separate so the service layer can tell whether the category was
	// human-assigned or inferred — relevant for dedup confidence decisions.
	AutoCategory Category
}

// EffectiveCategory returns Category if set, otherwise AutoCategory.
func (r ExtractionRequest) EffectiveCategory() Category {
	if r.Category != "" {
		return r.Category
	}
	return r.AutoCategory
}

// ExtractionResult is the full output of one extraction pass.
type ExtractionResult struct {
	Entities      []Entity
	Relationships []Relationship
	// Category is the effective category used during extraction, recorded for
	// provenance and to merge into Entity.Categories on write.
	Category Category
}

// EntityExtractor extracts named entities and typed relationships from video content.
type EntityExtractor interface {
	Extract(ctx context.Context, req ExtractionRequest) (ExtractionResult, error)
}

// EntityResolver deduplicates candidates against the global entity store using
// semantic similarity scoped by EntityType and Category.
type EntityResolver interface {
	Resolve(ctx context.Context, candidate Entity, category Category) (Entity, error)
	ResolveAll(ctx context.Context, candidates []Entity, category Category) ([]Entity, error)
}

// PromptBuilder constructs the LLM prompts for a specific domain category.
// Domain-specific examples are provided for universal entity types so the LLM
// knows what counts as a "tool" or "method" in that domain.
type PromptBuilder interface {
	Category() Category
	EntityPrompt(req ExtractionRequest) string
	RelationshipPrompt(req ExtractionRequest, entities []Entity) string
}
