package store

import (
	"context"
	"encoding/json"
	"fmt"

	"greenclaw/pkg/graphdb"
)

// VideoNode represents a video vertex in the knowledge graph.
type VideoNode struct {
	Key         string
	URL         string
	Title       string
	Description string
}

// EntityType classifies the kind of entity.
type EntityType string

const (
	EntityTypeTopic   EntityType = "topic"
	EntityTypeConcept EntityType = "concept"
)

// EntityNode represents a topic or concept vertex in the knowledge graph.
type EntityNode struct {
	Key  string
	Name string
	Type EntityType
}

const (
	colVideos   = "videos"
	colEntities = "entities"
	colMentions = "mentions"
	colRelated  = "related_to"
)

const colResults = "results"

// SaveResult persists a Result as a vertex, keyed by a hash of its URL.
func SaveResult(ctx context.Context, store graphdb.Store, result *Result) error {
	key := urlKey(result.URL)
	doc, err := resultToDoc(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	return store.UpsertVertex(ctx, colResults, key, doc)
}

// GetResult retrieves a previously saved Result by URL.
func GetResult(ctx context.Context, store graphdb.Store, url string) (*Result, error) {
	key := urlKey(url)
	var raw map[string]interface{}
	if err := store.GetVertex(ctx, colResults, key, &raw); err != nil {
		return nil, fmt.Errorf("get result vertex: %w", err)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal vertex: %w", err)
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}
	return &r, nil
}

func resultToDoc(r *Result) (map[string]interface{}, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// urlKey derives a stable ArangoDB-safe key from a URL.
func urlKey(url string) string {
	h := fmt.Sprintf("%x", simpleHash(url))
	return h
}

func simpleHash(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// UpsertVideo creates or updates a video vertex in the knowledge graph.
func UpsertVideo(ctx context.Context, store graphdb.Store, v VideoNode) error {
	return store.UpsertVertex(ctx, colVideos, v.Key, map[string]interface{}{
		"url":         v.URL,
		"title":       v.Title,
		"description": v.Description,
	})
}

// UpsertEntities creates or updates entity vertices in bulk.
func UpsertEntities(ctx context.Context, store graphdb.Store, nodes []EntityNode) error {
	if len(nodes) == 0 {
		return nil
	}
	docs := make([]map[string]interface{}, len(nodes))
	for i, n := range nodes {
		docs[i] = map[string]interface{}{
			"_key": n.Key,
			"name": n.Name,
			"type": string(n.Type),
		}
	}
	return store.BulkUpsertVertices(ctx, colEntities, docs)
}

// AddMentions creates video→entity edges (idempotent per video+entity pair).
func AddMentions(ctx context.Context, store graphdb.Store, videoKey string, entityKeys []string) error {
	if len(entityKeys) == 0 {
		return nil
	}
	edges := make([][2]string, len(entityKeys))
	for i, ek := range entityKeys {
		edges[i] = [2]string{colVideos + "/" + videoKey, colEntities + "/" + ek}
	}
	return store.BulkUpsertEdges(ctx, colMentions, edges)
}

// AddRelated increments entity↔entity co-mention edge weights for all unique
// pairs within entityKeys.
func AddRelated(ctx context.Context, store graphdb.Store, entityKeys []string) error {
	pairs := uniquePairs(entityKeys)
	if len(pairs) == 0 {
		return nil
	}
	return store.IncrementEdgePairs(ctx, colRelated, colEntities, pairs)
}

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
