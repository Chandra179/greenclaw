package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// JSONGenerator is the subset of llm.OllamaClient that entity extraction needs.
// Defined as an interface for testability.
type JSONGenerator interface {
	GenerateJSON(ctx context.Context, prompt string, schema json.RawMessage, numCtx int) (json.RawMessage, error)
}

// OllamaExtractor implements Extractor using an Ollama model.
type OllamaExtractor struct {
	client JSONGenerator
	numCtx int
}

// NewOllamaExtractor creates an extractor backed by the given Ollama client.
func NewOllamaExtractor(client JSONGenerator, numCtx int) *OllamaExtractor {
	return &OllamaExtractor{client: client, numCtx: numCtx}
}

func (e *OllamaExtractor) Extract(ctx context.Context, req ExtractionRequest) ([]Entity, error) {
	prompt := buildEntityPrompt(req)
	raw, err := e.client.GenerateJSON(ctx, prompt, entitySchema, e.numCtx)
	if err != nil {
		return nil, fmt.Errorf("entity extraction for %s: %w", req.VideoID, err)
	}
	return parseEntityResponse(raw)
}

func parseEntityResponse(raw json.RawMessage) ([]Entity, error) {
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

		et := EntityTypeTopic
		if item.Type == string(EntityTypeConcept) {
			et = EntityTypeConcept
		}
		entities = append(entities, Entity{Key: key, Name: name, Type: et})
	}

	return entities, nil
}
