package graph

import (
	"context"
	"fmt"
	"math"
)

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// EmbeddingGenerator produces a vector embedding for a given text string.
// Implemented by the Ollama client using a model such as nomic-embed-text.
type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
}

// SimilarEntityStore is the subset of the graph store needed by the resolver.
type SimilarEntityStore interface {
	// FindSimilarEntities returns up to limit entities matching the given type
	// and category whose stored embedding is closest to queryEmbedding.
	// Results are ordered by descending cosine similarity.
	FindSimilarEntities(ctx context.Context, entityType EntityType, category Category, queryEmbedding []float32, limit int) ([]ScoredEntity, error)

	// StoreEntityEmbedding persists the embedding for an existing entity key.
	StoreEntityEmbedding(ctx context.Context, key string, embedding []float32) error

	// MergeEntityCategory adds category to an entity's categories array if not
	// already present (idempotent).
	MergeEntityCategory(ctx context.Context, key string, category Category) error
}

// ScoredEntity pairs an entity with its cosine similarity to a query embedding.
type ScoredEntity struct {
	Entity
	Score float32
}

// ---------------------------------------------------------------------------
// Thresholds
// ---------------------------------------------------------------------------

const (
	// mergeThreshold: above this score the candidate is considered a duplicate
	// and the existing canonical entity is returned silently.
	mergeThreshold = float32(0.92)

	// reviewThreshold: between reviewThreshold and mergeThreshold the candidate
	// is merged conservatively but flagged for manual review.
	reviewThreshold = float32(0.80)

	// similarityQueryLimit: neighbours fetched per resolution lookup.
	similarityQueryLimit = 3
)

// ---------------------------------------------------------------------------
// EmbeddingEntityResolver
// ---------------------------------------------------------------------------

// EmbeddingEntityResolver implements EntityResolver using embedding cosine
// similarity scoped by EntityType and Category.
//
// Scoping by both type and category means:
//   - "kernel" (OS, tech_ai) and "kernel" (SVM, tech_ai) can still collide if
//     they share a type — which is correct, they should merge into one node.
//   - "kernel" (OS, tech_ai) and "kernel" (some unrelated finance use) won't
//     merge because the category scope differs.
//
// When a merge occurs the canonical entity gains the candidate's category via
// MergeEntityCategory, so cross-domain entities accumulate categories naturally.
type EmbeddingEntityResolver struct {
	embedder EmbeddingGenerator
	store    SimilarEntityStore
}

// NewEmbeddingEntityResolver constructs a resolver backed by the given dependencies.
func NewEmbeddingEntityResolver(embedder EmbeddingGenerator, store SimilarEntityStore) *EmbeddingEntityResolver {
	return &EmbeddingEntityResolver{embedder: embedder, store: store}
}

// Resolve deduplicates a single candidate entity against the global store.
// The category parameter is the effective category of the extraction request,
// used to scope the similarity search.
//
// On any store/embedder error the candidate is returned as-is so the pipeline
// is never blocked by resolution failures.
func (r *EmbeddingEntityResolver) Resolve(ctx context.Context, candidate Entity, category Category) (Entity, error) {
	embedding, err := r.embedder.GenerateEmbedding(ctx, candidate.Name)
	if err != nil {
		return candidate, fmt.Errorf("embed candidate %q: %w", candidate.Name, err)
	}
	candidate.Embedding = embedding

	neighbors, err := r.store.FindSimilarEntities(ctx, candidate.Type, category, embedding, similarityQueryLimit)
	if err != nil {
		return candidate, fmt.Errorf("find similar for %q: %w", candidate.Name, err)
	}

	for _, n := range neighbors {
		switch {
		case n.Score >= mergeThreshold:
			// High confidence duplicate: merge category onto the canonical entity.
			if mergeErr := r.store.MergeEntityCategory(ctx, n.Key, category); mergeErr != nil {
				// Non-fatal: the merge still proceeds.
				_ = mergeErr
			}
			return n.Entity, nil

		case n.Score >= reviewThreshold:
			// Medium confidence: merge conservatively but flag for review.
			logMergeReview(candidate, n)
			if mergeErr := r.store.MergeEntityCategory(ctx, n.Key, category); mergeErr != nil {
				_ = mergeErr
			}
			return n.Entity, nil
		}
	}

	// Genuinely new entity: persist its embedding for future resolutions.
	if storeErr := r.store.StoreEntityEmbedding(ctx, candidate.Key, embedding); storeErr != nil {
		return candidate, fmt.Errorf("store embedding for %q: %w", candidate.Key, storeErr)
	}

	return candidate, nil
}

// ResolveAll resolves each candidate in order. After resolution two candidates
// may collapse to the same canonical key — duplicates are dropped.
// The first non-nil error is returned alongside the best-effort result slice.
func (r *EmbeddingEntityResolver) ResolveAll(ctx context.Context, candidates []Entity, category Category) ([]Entity, error) {
	resolved := make([]Entity, 0, len(candidates))
	seenKeys := make(map[string]struct{}, len(candidates))
	var firstErr error

	for _, c := range candidates {
		entity, err := r.Resolve(ctx, c, category)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if _, dup := seenKeys[entity.Key]; dup {
			continue
		}
		seenKeys[entity.Key] = struct{}{}
		resolved = append(resolved, entity)
	}

	return resolved, firstErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func logMergeReview(candidate Entity, match ScoredEntity) {
	// Replace with a structured log or a write to a review_queue ArangoDB
	// collection so humans can confirm the merge.
	_ = candidate
	_ = match
}

// CosineSimilarity returns the cosine similarity between two equal-length vectors.
// Returns 0 if either vector has zero magnitude. Exported for tests and the
// ArangoDB adapter's in-process fallback path.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
