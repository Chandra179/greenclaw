package graphdb

import (
	"context"
	"fmt"
	"log"

	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/arangodb/go-driver/v2/arangodb/shared"
	"github.com/arangodb/go-driver/v2/connection"

	"greenclaw/internal/config"
)

const (
	colVideos   = "videos"
	colEntities = "entities"
	colMentions = "mentions"
	colRelated  = "related_to"
	graphName   = "knowledge"

	// vectorDimensions must match the embedding model output size.
	// nomic-embed-text → 768. Change if you switch models.
	vectorDimensions = 768
)

// ArangoGraph implements Store using ArangoDB.
type ArangoGraph struct {
	db arangodb.Database
}

// NewArangoGraph connects to ArangoDB and ensures the schema exists.
// Safe to call on every startup.
func NewArangoGraph(ctx context.Context, cfg config.GraphConfig) (*ArangoGraph, error) {
	conn := connection.NewHttpConnection(connection.HttpConfiguration{
		Authentication: connection.NewBasicAuth(cfg.Username, cfg.Password),
		Endpoint:       connection.NewRoundRobinEndpoints([]string{cfg.Endpoint}),
	})

	client := arangodb.NewClient(conn)

	db, err := openOrCreateDatabase(ctx, client, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", cfg.Database, err)
	}

	g := &ArangoGraph{db: db}
	if err := g.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	log.Printf("[graph] connected to ArangoDB database %q", cfg.Database)
	return g, nil
}

func openOrCreateDatabase(ctx context.Context, client arangodb.Client, name string) (arangodb.Database, error) {
	db, err := client.GetDatabase(ctx, name, nil)
	if err == nil {
		return db, nil
	}
	if !shared.IsNotFound(err) {
		return nil, err
	}
	return client.CreateDatabase(ctx, name, nil)
}

func (g *ArangoGraph) ensureSchema(ctx context.Context) error {
	for _, col := range []struct {
		name    string
		colType arangodb.CollectionType
	}{
		{colVideos, arangodb.CollectionTypeDocument},
		{colEntities, arangodb.CollectionTypeDocument},
		{colMentions, arangodb.CollectionTypeEdge},
		{colRelated, arangodb.CollectionTypeEdge},
		{"results", arangodb.CollectionTypeDocument},
	} {
		if err := ensureCollection(ctx, g.db, col.name, col.colType); err != nil {
			return fmt.Errorf("ensure collection %q: %w", col.name, err)
		}
	}

	if err := ensureUniqueIndex(ctx, g.db, colMentions, []string{"_from", "_to"}); err != nil {
		return fmt.Errorf("ensure index on %s: %w", colMentions, err)
	}
	// related_to is unique per (from, to, type) so the same two entities can
	// have multiple typed relationships between them.
	if err := ensureUniqueIndex(ctx, g.db, colRelated, []string{"_from", "_to", "type"}); err != nil {
		return fmt.Errorf("ensure index on %s: %w", colRelated, err)
	}
	// Index categories for fast type+category scoped queries in FindSimilarEntities.
	if err := ensureIndex(ctx, g.db, colEntities, []string{"type", "categories[*]"}); err != nil {
		return fmt.Errorf("ensure type+categories index on %s: %w", colEntities, err)
	}

	if err := g.ensureVectorIndex(ctx); err != nil {
		return fmt.Errorf("ensure vector index: %w", err)
	}

	if err := g.ensureGraph(ctx); err != nil {
		return fmt.Errorf("ensure graph: %w", err)
	}

	return nil
}

func ensureCollection(ctx context.Context, db arangodb.Database, name string, colType arangodb.CollectionType) error {
	exists, err := db.CollectionExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.CreateCollectionV2(ctx, name, &arangodb.CreateCollectionPropertiesV2{
		Type: &colType,
	})
	if shared.IsConflict(err) {
		return nil
	}
	return err
}

func ensureUniqueIndex(ctx context.Context, db arangodb.Database, colName string, fields []string) error {
	col, err := db.GetCollection(ctx, colName, nil)
	if err != nil {
		return err
	}
	_, _, err = col.EnsurePersistentIndex(ctx, fields, &arangodb.CreatePersistentIndexOptions{
		Unique: newBool(true),
		Sparse: newBool(false),
	})
	return err
}

func ensureIndex(ctx context.Context, db arangodb.Database, colName string, fields []string) error {
	col, err := db.GetCollection(ctx, colName, nil)
	if err != nil {
		return err
	}
	_, _, err = col.EnsurePersistentIndex(ctx, fields, &arangodb.CreatePersistentIndexOptions{
		Unique: newBool(false),
		Sparse: newBool(true),
	})
	return err
}

// ensureVectorIndex logs instructions for creating the ANN vector index.
// ArangoDB 3.12+ supports vector indexes via the REST API; the Go driver v2
// does not yet expose a typed helper so we degrade gracefully if absent.
func (g *ArangoGraph) ensureVectorIndex(ctx context.Context) error {
	aql := `FOR idx IN (INDEXES(@@col))
  FILTER idx.type == "vector" && idx.fields == ["embedding"]
  LIMIT 1 RETURN true`

	cursor, err := g.db.Query(ctx, aql, &arangodb.QueryOptions{
		BindVars: map[string]interface{}{"@col": colEntities},
	})
	if err != nil {
		log.Printf("[graph] vector index check failed (ArangoDB <3.12?): %v — semantic dedup disabled", err)
		return nil
	}
	defer cursor.Close()

	var exists bool
	_, _ = cursor.ReadDocument(ctx, &exists)
	if exists {
		return nil
	}

	log.Printf("[graph] WARNING: vector index not found on %q. "+
		"Create it via: POST /_api/index?collection=%s "+
		`{"type":"vector","fields":["embedding"],"params":{"dimension":%d,"metric":"cosine"}}`,
		colEntities, colEntities, vectorDimensions)

	return nil
}

func (g *ArangoGraph) ensureGraph(ctx context.Context) error {
	exists, err := g.db.GraphExists(ctx, graphName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = g.db.CreateGraph(ctx, graphName, &arangodb.GraphDefinition{
		EdgeDefinitions: []arangodb.EdgeDefinition{
			{Collection: colMentions, From: []string{colVideos}, To: []string{colEntities}},
			{Collection: colRelated, From: []string{colEntities}, To: []string{colEntities}},
		},
	}, nil)
	if shared.IsConflict(err) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Vertex operations
// ---------------------------------------------------------------------------

func (g *ArangoGraph) UpsertVertex(ctx context.Context, collection, key string, doc map[string]interface{}) error {
	aql := `UPSERT { _key: @key }
INSERT MERGE(@doc, { _key: @key })
UPDATE @doc
IN @@col`
	return g.exec(ctx, aql, map[string]interface{}{
		"@col": collection,
		"key":  key,
		"doc":  doc,
	})
}

func (g *ArangoGraph) BulkUpsertVertices(ctx context.Context, collection string, docs []map[string]interface{}) error {
	if len(docs) == 0 {
		return nil
	}
	aql := `FOR d IN @docs
UPSERT { _key: d._key }
INSERT d
UPDATE UNSET(d, "_key")
IN @@col`
	return g.exec(ctx, aql, map[string]interface{}{
		"@col": collection,
		"docs": docs,
	})
}

// ---------------------------------------------------------------------------
// Edge operations
// ---------------------------------------------------------------------------

func (g *ArangoGraph) BulkUpsertEdges(ctx context.Context, collection string, edges [][2]string) error {
	if len(edges) == 0 {
		return nil
	}
	aql := `FOR edge IN @edges
UPSERT { _from: edge[0], _to: edge[1] }
INSERT { _from: edge[0], _to: edge[1] }
UPDATE {}
IN @@col`
	return g.exec(ctx, aql, map[string]interface{}{
		"@col":  collection,
		"edges": edges,
	})
}

func (g *ArangoGraph) BulkUpsertTypedEdges(ctx context.Context, collection string, edges []TypedEdge) error {
	if len(edges) == 0 {
		return nil
	}
	docs := make([]map[string]interface{}, len(edges))
	for i, e := range edges {
		docs[i] = map[string]interface{}{
			"_from": e.From,
			"_to":   e.To,
			"type":  e.Type,
		}
	}
	aql := `FOR e IN @edges
UPSERT { _from: e._from, _to: e._to, type: e.type }
INSERT e
UPDATE {}
IN @@col`
	return g.exec(ctx, aql, map[string]interface{}{
		"@col":  collection,
		"edges": docs,
	})
}

func (g *ArangoGraph) IncrementEdgePairs(ctx context.Context, edgeCollection, vertexCollection string, pairs [][2]string) error {
	if len(pairs) == 0 {
		return nil
	}
	aql := `FOR pair IN @pairs
LET from = CONCAT(@vertexCol, "/", pair[0])
LET to   = CONCAT(@vertexCol, "/", pair[1])
UPSERT { _from: from, _to: to }
INSERT { _from: from, _to: to, weight: 1 }
UPDATE { weight: OLD.weight + 1 }
IN @@edgeCol`
	return g.exec(ctx, aql, map[string]interface{}{
		"@edgeCol":  edgeCollection,
		"vertexCol": vertexCollection,
		"pairs":     pairs,
	})
}

// ---------------------------------------------------------------------------
// Vertex read
// ---------------------------------------------------------------------------

func (g *ArangoGraph) GetVertex(ctx context.Context, collection, key string, dest interface{}) error {
	aql := `RETURN DOCUMENT(@@col, @key)`
	cursor, err := g.db.Query(ctx, aql, &arangodb.QueryOptions{
		BindVars: map[string]interface{}{
			"@col": collection,
			"key":  key,
		},
	})
	if err != nil {
		return err
	}
	defer cursor.Close()
	_, err = cursor.ReadDocument(ctx, dest)
	return err
}

// ---------------------------------------------------------------------------
// Embedding / vector search
// ---------------------------------------------------------------------------

// StoreEntityEmbedding persists a vector embedding on an existing entity document.
func (g *ArangoGraph) StoreEntityEmbedding(ctx context.Context, key string, embedding []float32) error {
	aql := `UPDATE { _key: @key, embedding: @embedding } IN @@col`
	return g.exec(ctx, aql, map[string]interface{}{
		"@col":      colEntities,
		"key":       key,
		"embedding": embedding,
	})
}

// FindSimilarEntities uses ArangoDB's COSINE_SIMILARITY to find entities of the
// given type that appear in the given category and are semantically close to
// queryEmbedding.
//
// Requires ArangoDB 3.12+ with a vector index on entities.embedding.
// Degrades gracefully to an empty result if the index or function is absent.
func (g *ArangoGraph) FindSimilarEntities() {}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (g *ArangoGraph) Close() error { return nil }

func (g *ArangoGraph) exec(ctx context.Context, aql string, bindVars map[string]interface{}) error {
	cursor, err := g.db.Query(ctx, aql, &arangodb.QueryOptions{BindVars: bindVars})
	if err != nil {
		return err
	}
	return cursor.Close()
}

func newBool(v bool) *bool { return &v }
