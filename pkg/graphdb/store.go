package graphdb

import (
	"context"
)

// Client is the interface for graph storage backends.
type Client interface {
	// UpsertNode creates or updates a node by label and key.
	// props is merged into the node on update.
	UpsertNode(ctx context.Context, label, key string, props map[string]interface{}) error

	// BulkUpsertNodes creates or updates multiple nodes of the same label.
	// Each map in nodes must include a "key" field used as the unique identifier.
	BulkUpsertNodes(ctx context.Context, label string, nodes []map[string]interface{}) error

	// BulkUpsertRelationships creates or updates typed directed relationships.
	// Idempotent by (FromLabel, FromKey, Type, ToLabel, ToKey).
	BulkUpsertRelationships(ctx context.Context, rels []Relationship) error

	// GetNode retrieves a node by label and key, unmarshalling its properties into dest.
	GetNode(ctx context.Context, label, key string, dest interface{}) error

	// StoreEmbedding persists an embedding vector on an existing node.
	StoreEmbedding(ctx context.Context, label, key string, embedding []float32) error

	// Close releases resources held by the backend.
	Close() error
}

// Relationship is a typed, directed edge between two nodes.
type Relationship struct {
	FromLabel string
	FromKey   string
	Type      string
	ToLabel   string
	ToKey     string
}
