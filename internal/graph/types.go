package graph

import (
	"context"
	"encoding/json"

	"greenclaw/pkg/llm"
)

// llmClient is the subset of llm.OllamaClient used by this package.
// Defined locally so graph stages don't depend on the concrete type.
type llmClient interface {
	Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
}

// RawEntity is an entity as extracted by the LLM before resolution.
type RawEntity struct {
	Name string
	Type string
}

// RawTriple is a subject-predicate-object triple before entity resolution.
type RawTriple struct {
	Subject   string
	Predicate string
	Object    string
}

// Entity is a deduplicated, canonical entity ready to write to the graph.
type Entity struct {
	Key      string // slug used as the unique node key
	Name     string // canonical display name
	Type     string // from the domain ontology
	Category string
}

// Triple is a resolved triple whose Subject/Object map to Entity.Key values.
type Triple struct {
	FromKey   string
	Predicate string
	ToKey     string
}

// WeightedTriple is a Triple with an occurrence count across all chunks.
type WeightedTriple struct {
	Triple
	Weight int
}

// extractionOutput is the JSON shape returned by the LLM during extraction.
type extractionOutput struct {
	Entities []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"entities"`
	Relationships []struct {
		Subject   string `json:"subject"`
		Predicate string `json:"predicate"`
		Object    string `json:"object"`
	} `json:"relationships"`
}

// resolutionOutput is the JSON shape returned by the LLM during entity resolution.
type resolutionOutput struct {
	Groups []struct {
		Canonical string   `json:"canonical"`
		Aliases   []string `json:"aliases"`
	} `json:"groups"`
}

// categoryOntology maps a video category to its domain-specific entity types.
var categoryOntology = map[string][]string{
	"technology": {"Model", "Framework", "Company", "Researcher", "Benchmark", "Concept"},
	"economy":    {"Policy", "Institution", "Metric", "Country", "Event", "Person"},
}

func ontologyFor(category string) []string {
	if types, ok := categoryOntology[category]; ok {
		return types
	}
	return []string{"Concept", "Person", "Organization", "Event"}
}

// extractionSchema is the JSON schema enforced on the LLM extraction output.
var extractionSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "entities": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "type": { "type": "string" }
        },
        "required": ["name", "type"]
      }
    },
    "relationships": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "subject":   { "type": "string" },
          "predicate": { "type": "string" },
          "object":    { "type": "string" }
        },
        "required": ["subject", "predicate", "object"]
      }
    }
  },
  "required": ["entities", "relationships"]
}`)

// resolutionSchema is the JSON schema enforced on the LLM resolution output.
var resolutionSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "groups": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "canonical": { "type": "string" },
          "aliases":   { "type": "array", "items": { "type": "string" } }
        },
        "required": ["canonical", "aliases"]
      }
    }
  },
  "required": ["groups"]
}`)
