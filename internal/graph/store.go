package graph

import (
	"context"
	"fmt"

	"greenclaw/pkg/graphdb"
)

// Store writes entities as nodes and weighted triples as relationships to the
// graph DB. It only writes edges for entities that exist in the entity list.
func Store(ctx context.Context, db graphdb.Client, videoID string, entities []Entity, triples []WeightedTriple) (entitiesWritten, edgesWritten int, err error) {
	// Build a set of known entity keys for edge validation.
	knownKeys := make(map[string]struct{}, len(entities))
	for _, e := range entities {
		knownKeys[e.Key] = struct{}{}
	}

	// Convert Entity slice to the generic map format expected by BulkUpsertNodes.
	docs := make([]map[string]interface{}, 0, len(entities))
	for _, e := range entities {
		docs = append(docs, map[string]interface{}{
			"key":      e.Key,
			"name":     e.Name,
			"type":     e.Type,
			"category": e.Category,
		})
	}

	if err := db.BulkUpsertNodes(ctx, "Entity", docs); err != nil {
		return 0, 0, fmt.Errorf("upsert entities: %w", err)
	}

	// Build relationships, skipping edges whose endpoints are unknown.
	rels := make([]graphdb.Relationship, 0, len(triples))
	for _, wt := range triples {
		if _, ok := knownKeys[wt.FromKey]; !ok {
			continue
		}
		if _, ok := knownKeys[wt.ToKey]; !ok {
			continue
		}
		rels = append(rels, graphdb.Relationship{
			FromLabel: "Entity",
			FromKey:   wt.FromKey,
			Type:      wt.Predicate,
			ToLabel:   "Entity",
			ToKey:     wt.ToKey,
			Weight:    wt.Weight,
		})
	}

	if len(rels) > 0 {
		if err := db.BulkUpsertRelationships(ctx, rels); err != nil {
			return len(docs), 0, fmt.Errorf("upsert relationships: %w", err)
		}
	}

	return len(docs), len(rels), nil
}
