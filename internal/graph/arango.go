package graph

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

// ArangoGraph implements KnowledgeGraph using ArangoDB.
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
	// Ensure vertex and edge collections exist.
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

	// Ensure unique indexes for idempotent edge upserts.
	if err := ensureUniqueIndex(ctx, g.db, colMentions, []string{"_from", "_to"}); err != nil {
		return fmt.Errorf("ensure index on %s: %w", colMentions, err)
	}
	if err := ensureUniqueIndex(ctx, g.db, colRelated, []string{"_from", "_to"}); err != nil {
		return fmt.Errorf("ensure index on %s: %w", colRelated, err)
	}

	// Ensure named graph.
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
		return nil // created concurrently
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

// UpsertVideo creates or updates a video vertex.
func (g *ArangoGraph) UpsertVideo(ctx context.Context, v VideoNode) error {
	aql := `UPSERT { _key: @key }
INSERT { _key: @key, url: @url, title: @title, description: @description }
UPDATE { title: @title, description: @description }
IN @@col`
	return g.exec(ctx, aql, map[string]interface{}{
		"@col":        colVideos,
		"key":         v.Key,
		"url":         v.URL,
		"title":       v.Title,
		"description": v.Description,
	})
}

// UpsertEntities creates or updates entity vertices in bulk.
func (g *ArangoGraph) UpsertEntities(ctx context.Context, nodes []EntityNode) error {
	if len(nodes) == 0 {
		return nil
	}
	aql := `FOR e IN @entities
UPSERT { _key: e.key }
INSERT { _key: e.key, name: e.name, type: e.type }
UPDATE { name: e.name }
IN @@col`
	docs := make([]map[string]interface{}, len(nodes))
	for i, n := range nodes {
		docs[i] = map[string]interface{}{
			"key":  n.Key,
			"name": n.Name,
			"type": string(n.Type),
		}
	}
	return g.exec(ctx, aql, map[string]interface{}{
		"@col":     colEntities,
		"entities": docs,
	})
}

// AddMentions creates video→entity edges (idempotent per pair).
func (g *ArangoGraph) AddMentions(ctx context.Context, videoKey string, entityKeys []string) error {
	if len(entityKeys) == 0 {
		return nil
	}
	aql := `FOR ek IN @entityKeys
LET from = CONCAT(@videoCol, "/", @videoKey)
LET to   = CONCAT(@entityCol, "/", ek)
UPSERT { _from: from, _to: to }
INSERT { _from: from, _to: to }
UPDATE {}
IN @@col`
	return g.exec(ctx, aql, map[string]interface{}{
		"@col":       colMentions,
		"videoCol":   colVideos,
		"videoKey":   videoKey,
		"entityCol":  colEntities,
		"entityKeys": entityKeys,
	})
}

// AddRelated increments (or creates) entity↔entity co-mention edges for all
// unique unordered pairs in entityKeys.
func (g *ArangoGraph) AddRelated(ctx context.Context, entityKeys []string) error {
	pairs := uniquePairs(entityKeys)
	if len(pairs) == 0 {
		return nil
	}
	aql := `FOR pair IN @pairs
LET from = CONCAT(@col, "/", pair[0])
LET to   = CONCAT(@col, "/", pair[1])
UPSERT { _from: from, _to: to }
INSERT { _from: from, _to: to, weight: 1 }
UPDATE { weight: OLD.weight + 1 }
IN @@edgeCol`
	return g.exec(ctx, aql, map[string]interface{}{
		"@edgeCol": colRelated,
		"col":      colEntities,
		"pairs":    pairs,
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

// uniquePairs returns all unique unordered pairs from keys, with each pair
// sorted lexicographically so (a,b) and (b,a) produce the same pair.
func uniquePairs(keys []string) [][2]string {
	var pairs [][2]string
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			a, b := keys[i], keys[j]
			if a > b {
				a, b = b, a
			}
			pairs = append(pairs, [2]string{a, b})
		}
	}
	return pairs
}

func newBool(v bool) *bool { return &v }
