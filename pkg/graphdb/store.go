package graphdb

import "context"

// Store is the interface for graph storage backends.
type Store interface {
	// UpsertVertex creates or updates a vertex document by key.
	UpsertVertex(ctx context.Context, collection, key string, doc map[string]interface{}) error
	// BulkUpsertVertices creates or updates multiple vertex documents.
	// Each doc must include a "_key" field.
	BulkUpsertVertices(ctx context.Context, collection string, docs []map[string]interface{}) error
	// BulkUpsertEdges creates or updates edges (idempotent by from+to pair).
	// Each edge is [from, to] in "collection/key" format.
	BulkUpsertEdges(ctx context.Context, collection string, edges [][2]string) error
	// IncrementEdgePairs creates or increments a weight field on edges for all
	// pairs, where each vertex key is prefixed with vertexCollection.
	IncrementEdgePairs(ctx context.Context, edgeCollection, vertexCollection string, pairs [][2]string) error
	// Close releases resources held by the backend.
	Close() error
}
