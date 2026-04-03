package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type JSONGenerator interface {
	GenerateJSON(ctx context.Context, prompt string, schema json.RawMessage, numCtx int) (json.RawMessage, error)
}

type OllamaEntityExtractor struct {
	client   JSONGenerator
	registry *PromptBuilderRegistry
	numCtx   int
}

func NewOllamaEntityExtractor(
	client JSONGenerator,
	numCtx int,
) *OllamaEntityExtractor {
	registry := NewPromptBuilderRegistry()
	return &OllamaEntityExtractor{
		client:   client,
		registry: registry,
		numCtx:   numCtx,
	}
}

// Extract runs the full extraction pipeline for one video:
// classify → entity extraction → relationship extraction.
//
// Relationship extraction failure is non-fatal: the result will contain
// entities with an empty relationship list and the error will be logged.
func (e *OllamaEntityExtractor) Extract(ctx context.Context, req ExtractionRequest) (ExtractionResult, error) {
	category := req.EffectiveCategory()
	builder := e.registry.For(req.Category)

	// Step 2: extract entities.
	entities, err := e.extractEntities(ctx, req, builder)
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("entity extraction for %s: %w", req.VideoID, err)
	}

	if len(entities) == 0 {
		return ExtractionResult{Category: category}, nil
	}

	// Step 3: extract relationships (best-effort).
	relationships, err := e.extractRelationships(ctx, req, builder, entities)
	if err != nil {
		log.Printf("[graph] relationship extraction failed for %s: %v", req.VideoID, err)
	}

	return ExtractionResult{
		Entities:      entities,
		Relationships: relationships,
		Category:      category,
	}, nil
}

// ---------------------------------------------------------------------------
// Step 2 – entity extraction
// ---------------------------------------------------------------------------

func (e *OllamaEntityExtractor) extractEntities(ctx context.Context, req ExtractionRequest, builder PromptBuilder) ([]Entity, error) {
	prompt := builder.EntityPrompt(req)
	raw, err := e.client.GenerateJSON(ctx, prompt, entitySchema, e.numCtx)
	if err != nil {
		return nil, err
	}
	return parseEntityResponse(raw, req.EffectiveCategory())
}

func parseEntityResponse(raw json.RawMessage, category Category) ([]Entity, error) {
	var resp entityListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse entity response: %w", err)
	}

	entities := make([]Entity, 0, len(resp.Entities))
	seen := make(map[string]struct{}, len(resp.Entities))

	for _, item := range resp.Entities {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		key := normaliseKey(name)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		et, ok := validEntityTypes[item.Type]
		if !ok {
			et = EntityTypeConcept // safe fallback
		}

		entities = append(entities, Entity{
			Key:        key,
			Name:       name,
			Type:       et,
			Categories: []Category{category},
		})
	}

	return entities, nil
}

// ---------------------------------------------------------------------------
// Step 3 – relationship extraction
// ---------------------------------------------------------------------------

func (e *OllamaEntityExtractor) extractRelationships(ctx context.Context, req ExtractionRequest, builder PromptBuilder, entities []Entity) ([]Relationship, error) {
	if len(entities) < 2 {
		return nil, nil
	}

	prompt := builder.RelationshipPrompt(req, entities)
	raw, err := e.client.GenerateJSON(ctx, prompt, relationshipSchema, e.numCtx)
	if err != nil {
		return nil, err
	}
	return parseRelationshipResponse(raw, entities)
}

func parseRelationshipResponse(raw json.RawMessage, entities []Entity) ([]Relationship, error) {
	var resp relationshipListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse relationship response: %w", err)
	}

	// name → key lookup to validate that the LLM only references known entities.
	nameToKey := make(map[string]string, len(entities))
	for _, e := range entities {
		nameToKey[strings.ToLower(e.Name)] = e.Key
	}

	type edgeID struct{ from, to string }
	seen := make(map[edgeID]struct{})
	relationships := make([]Relationship, 0, len(resp.Relationships))

	for _, item := range resp.Relationships {
		fromKey, okFrom := nameToKey[strings.ToLower(strings.TrimSpace(item.From))]
		toKey, okTo := nameToKey[strings.ToLower(strings.TrimSpace(item.To))]
		if !okFrom || !okTo {
			continue // LLM hallucinated a name not in our list
		}
		if fromKey == toKey {
			continue // self-loop
		}

		rt, ok := validRelationshipTypes[item.Type]
		if !ok {
			continue
		}

		id := edgeID{fromKey, toKey}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		relationships = append(relationships, Relationship{
			FromKey: fromKey,
			ToKey:   toKey,
			Type:    rt,
		})
	}

	return relationships, nil
}
