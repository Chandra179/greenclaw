package graph

import (
	"context"

	"greenclaw/internal/entity"
)

// VideoNode represents a video vertex in the knowledge graph.
type VideoNode struct {
	Key         string `json:"_key"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// EntityNode represents a topic or concept vertex in the knowledge graph.
type EntityNode struct {
	Key  string          `json:"_key"`
	Name string          `json:"name"`
	Type entity.EntityType `json:"type"`
}

// KnowledgeGraph is the interface for graph storage backends.
type KnowledgeGraph interface {
	// UpsertVideo creates or updates a video vertex.
	UpsertVideo(ctx context.Context, v VideoNode) error
	// UpsertEntities creates or updates entity vertices in bulk.
	UpsertEntities(ctx context.Context, nodes []EntityNode) error
	// AddMentions creates video→entity edges (idempotent per video+entity pair).
	AddMentions(ctx context.Context, videoKey string, entityKeys []string) error
	// AddRelated increments (or creates) entity→entity co-mention edges for all
	// unique pairs within entityKeys.
	AddRelated(ctx context.Context, entityKeys []string) error
	// Close releases resources held by the backend.
	Close() error
}
