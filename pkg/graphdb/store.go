package graphdb

import (
	"context"
)

// Client is the interface for graph storage backends.
type Client interface {
	// UpsertVertex creates or updates a vertex document by key.
	UpsertVertex(ctx context.Context, collection, key string, doc map[string]interface{}) error

	// BulkUpsertVertices creates or updates multiple vertex documents.
	// Each doc must include a "_key" field.
	BulkUpsertVertices(ctx context.Context, collection string, docs []map[string]interface{}) error

	// BulkUpsertEdges creates or updates untyped edges, idempotent by from+to pair.
	// Each edge is [from, to] in "collection/key" format.
	BulkUpsertEdges(ctx context.Context, collection string, edges [][2]string) error

	// BulkUpsertTypedEdges creates or updates typed, directed edges.
	// Idempotent by (from, to, type) triple.
	BulkUpsertTypedEdges(ctx context.Context, collection string, edges []TypedEdge) error

	// IncrementEdgePairs creates or increments a weight field on edges for all
	// pairs. Each vertex key is prefixed with vertexCollection to form the full ID.
	IncrementEdgePairs(ctx context.Context, edgeCollection, vertexCollection string, pairs [][2]string) error

	// GetVertex retrieves a vertex document by key into dest.
	GetVertex(ctx context.Context, collection, key string, dest interface{}) error

	// StoreEntityEmbedding persists an embedding vector on an existing entity document.
	// Implements graph.SimilarEntityStore.
	StoreEntityEmbedding(ctx context.Context, key string, embedding []float32) error

	// Close releases resources held by the backend.
	Close() error
}

// TypedEdge is a directed, typed edge between two vertex IDs.
type TypedEdge struct {
	// From and To are full vertex IDs in "collection/key" format.
	From string
	To   string
	// Type is the relationship label (e.g. "extends", "implements").
	Type string
}
