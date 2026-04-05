package graphdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"greenclaw/internal/config"
)

// Neo4jGraph implements Client using Neo4j.
type Neo4jGraph struct {
	driver neo4j.DriverWithContext
	db     string
}

// NewNeo4jGraph connects to Neo4j and ensures constraints exist.
// Safe to call on every startup.
func NewNeo4jGraph(ctx context.Context, cfg config.GraphConfig) (*Neo4jGraph, error) {
	driver, err := neo4j.NewDriverWithContext(cfg.URI, neo4j.NoAuth())
	if err != nil {
		return nil, fmt.Errorf("create neo4j driver: %w", err)
	}

	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("connect to neo4j at %q: %w", cfg.URI, err)
	}

	g := &Neo4jGraph{driver: driver, db: cfg.Database}
	if err := g.ensureConstraints(ctx); err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("ensure constraints: %w", err)
	}

	log.Printf("[graph] connected to Neo4j at %q (db=%q)", cfg.URI, cfg.Database)
	return g, nil
}

func (g *Neo4jGraph) ensureConstraints(ctx context.Context) error {
	stmts := []string{
		`CREATE CONSTRAINT video_key IF NOT EXISTS FOR (n:Video) REQUIRE n.key IS UNIQUE`,
		`CREATE CONSTRAINT entity_key IF NOT EXISTS FOR (n:Entity) REQUIRE n.key IS UNIQUE`,
	}
	session := g.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: g.db})
	defer session.Close(ctx)

	for _, stmt := range stmts {
		if _, err := session.Run(ctx, stmt, nil); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Node operations
// ---------------------------------------------------------------------------

func (g *Neo4jGraph) UpsertNode(ctx context.Context, label, key string, props map[string]interface{}) error {
	cypher := fmt.Sprintf(`MERGE (n:%s {key: $key}) SET n += $props`, label)
	return g.exec(ctx, cypher, map[string]interface{}{
		"key":   key,
		"props": props,
	})
}

func (g *Neo4jGraph) BulkUpsertNodes(ctx context.Context, label string, nodes []map[string]interface{}) error {
	if len(nodes) == 0 {
		return nil
	}
	cypher := fmt.Sprintf(`UNWIND $nodes AS n
MERGE (node:%s {key: n.key})
SET node += n`, label)
	return g.exec(ctx, cypher, map[string]interface{}{"nodes": nodes})
}

// ---------------------------------------------------------------------------
// Relationship operations
// ---------------------------------------------------------------------------

func (g *Neo4jGraph) BulkUpsertRelationships(ctx context.Context, rels []Relationship) error {
	if len(rels) == 0 {
		return nil
	}

	// Group by (fromLabel, relType, toLabel) so we can use UNWIND per group.
	type groupKey struct{ fromLabel, relType, toLabel string }
	groups := map[groupKey][]map[string]interface{}{}
	for _, r := range rels {
		k := groupKey{r.FromLabel, r.Type, r.ToLabel}
		w := r.Weight
		if w <= 0 {
			w = 1
		}
		groups[k] = append(groups[k], map[string]interface{}{
			"fromKey": r.FromKey,
			"toKey":   r.ToKey,
			"weight":  w,
		})
	}

	session := g.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: g.db})
	defer session.Close(ctx)

	for k, pairs := range groups {
		cypher := fmt.Sprintf(`UNWIND $pairs AS p
MATCH (a:%s {key: p.fromKey}), (b:%s {key: p.toKey})
MERGE (a)-[r:%s]->(b)
ON CREATE SET r.weight = p.weight
ON MATCH SET r.weight = coalesce(r.weight, 0) + p.weight`, k.fromLabel, k.toLabel, k.relType)
		if _, err := session.Run(ctx, cypher, map[string]interface{}{"pairs": pairs}); err != nil {
			return fmt.Errorf("upsert (%s)-[%s]->(%s): %w", k.fromLabel, k.relType, k.toLabel, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Node read
// ---------------------------------------------------------------------------

func (g *Neo4jGraph) GetNode(ctx context.Context, label, key string, dest interface{}) error {
	cypher := fmt.Sprintf(`MATCH (n:%s {key: $key}) RETURN properties(n) AS props LIMIT 1`, label)
	session := g.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: g.db})
	defer session.Close(ctx)

	result, err := session.Run(ctx, cypher, map[string]interface{}{"key": key})
	if err != nil {
		return err
	}
	record, err := result.Single(ctx)
	if err != nil {
		return fmt.Errorf("%s{key:%q} not found: %w", label, key, err)
	}

	raw, _ := record.Get("props")
	b, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal node props: %w", err)
	}
	return json.Unmarshal(b, dest)
}

// ---------------------------------------------------------------------------
// Embedding
// ---------------------------------------------------------------------------

func (g *Neo4jGraph) StoreEmbedding(ctx context.Context, label, key string, embedding []float32) error {
	cypher := fmt.Sprintf(`MATCH (n:%s {key: $key}) SET n.embedding = $embedding`, label)
	return g.exec(ctx, cypher, map[string]interface{}{
		"key":       key,
		"embedding": embedding,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (g *Neo4jGraph) Close() error {
	return g.driver.Close(context.Background())
}

func (g *Neo4jGraph) exec(ctx context.Context, cypher string, params map[string]interface{}) error {
	session := g.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: g.db})
	defer session.Close(ctx)
	_, err := session.Run(ctx, cypher, params)
	return err
}
