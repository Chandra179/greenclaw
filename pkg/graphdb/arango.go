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
)

// ArangoGraph implements Store using ArangoDB.
type ArangoGraph struct {
	db arangodb.Database
}

// NewArangoGraph connects to ArangoDB and ensures the schema (collections, named
// graph, indexes) exists. Safe to call on every startup.
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
	} {
		if err := ensureCollection(ctx, g.db, col.name, col.colType); err != nil {
			return fmt.Errorf("ensure collection %q: %w", col.name, err)
		}
	}

	if err := ensureUniqueIndex(ctx, g.db, colMentions, []string{"_from", "_to"}); err != nil {
		return fmt.Errorf("ensure index on %s: %w", colMentions, err)
	}
	if err := ensureUniqueIndex(ctx, g.db, colRelated, []string{"_from", "_to"}); err != nil {
		return fmt.Errorf("ensure index on %s: %w", colRelated, err)
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

// UpsertVertex creates or updates a vertex document by key.
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

// BulkUpsertVertices creates or updates multiple vertex documents.
// Each doc must include a "_key" field.
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

// BulkUpsertEdges creates or updates edges (idempotent by from+to pair).
// Each edge is [from, to] in "collection/key" format.
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

// IncrementEdgePairs creates or increments a weight field on edges for all pairs.
// Each vertex key is prefixed with vertexCollection to form the full vertex ID.
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

// Close is a no-op; the HTTP connection has no persistent state to release.
func (g *ArangoGraph) Close() error { return nil }

// exec runs a fire-and-forget AQL query (no result needed).
func (g *ArangoGraph) exec(ctx context.Context, aql string, bindVars map[string]interface{}) error {
	cursor, err := g.db.Query(ctx, aql, &arangodb.QueryOptions{BindVars: bindVars})
	if err != nil {
		return err
	}
	return cursor.Close()
}

func newBool(v bool) *bool { return &v }
