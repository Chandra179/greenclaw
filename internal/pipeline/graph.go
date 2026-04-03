package pipeline

import (
	"context"

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
