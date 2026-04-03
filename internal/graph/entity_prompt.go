package graph

import (
	"encoding/json"
	"regexp"
	"strings"
)

type entityListResponse struct {
	Entities []entityItem `json:"entities"`
}

type entityItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type relationshipListResponse struct {
	Relationships []relationshipItem `json:"relationships"`
}

type relationshipItem struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// ---------------------------------------------------------------------------
// JSON schemas
// ---------------------------------------------------------------------------

// entitySchema is the JSON schema passed to Ollama's structured output format.
// Uses the universal 7-type system defined in entity.go.
var entitySchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "entities": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "type": {
            "type": "string",
            "enum": ["person", "organization", "tool", "method", "concept", "work", "metric"]
          }
        },
        "required": ["name", "type"]
      }
    }
  },
  "required": ["entities"]
}`)

// relationshipSchema is the JSON schema for the relationship extraction call.
var relationshipSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "relationships": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "from": {"type": "string"},
          "to":   {"type": "string"},
          "type": {
            "type": "string",
            "enum": ["extends", "implements", "optimizes", "used_for", "part_of", "compares_to", "introduced_by"]
          }
        },
        "required": ["from", "to", "type"]
      }
    }
  },
  "required": ["relationships"]
}`)

// ---------------------------------------------------------------------------
// Type allow-lists (used during response parsing)
// ---------------------------------------------------------------------------

var validEntityTypes = map[string]EntityType{
	"person":       EntityTypePerson,
	"organization": EntityTypeOrganization,
	"tool":         EntityTypeTool,
	"method":       EntityTypeMethod,
	"concept":      EntityTypeConcept,
	"work":         EntityTypeWork,
	"metric":       EntityTypeMetric,
}

var validRelationshipTypes = map[string]RelationshipType{
	"extends":       RelExtends,
	"implements":    RelImplements,
	"optimizes":     RelOptimizes,
	"used_for":      RelUsedFor,
	"part_of":       RelPartOf,
	"compares_to":   RelComparesTo,
	"introduced_by": RelIntroducedBy,
}

// ---------------------------------------------------------------------------
// Key normalisation
// ---------------------------------------------------------------------------

// canonicalAliases is a seed set of well-known abbreviation→canonical mappings.
// This table intentionally stays small. The resolver's self-learning alias store
// (ArangoDB entity_aliases collection) is the primary dedup mechanism and grows
// automatically from resolution events without code changes.
//
// Entries here cover only unambiguous, cross-domain abbreviations where the LLM
// reliably produces the short form despite the prompt's instructions.
var canonicalAliases = map[string]string{
	// Universal / cross-domain
	"ai":  "artificial_intelligence",
	"ml":  "machine_learning",
	"nlp": "natural_language_processing",
	"llm": "large_language_model",
	"dl":  "deep_learning",
	"rl":  "reinforcement_learning",
	"cv":  "computer_vision",
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// normaliseKey converts a display name to a stable ArangoDB-safe vertex key.
// Steps: lowercase → collapse non-alphanumeric runs to "_" → trim underscores
// → apply canonical alias map → truncate to 240 chars.
func normaliseKey(name string) string {
	key := strings.ToLower(name)
	key = nonAlphanumRe.ReplaceAllString(key, "_")
	key = strings.Trim(key, "_")

	if alias, ok := canonicalAliases[key]; ok {
		key = alias
	}

	if len(key) > 240 {
		key = key[:240]
	}
	return key
}

// truncate shortens s to at most n characters, appending "..." if cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
