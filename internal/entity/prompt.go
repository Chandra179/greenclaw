package entity

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// entityListResponse is the expected structured output from the LLM.
type entityListResponse struct {
	Entities []entityItem `json:"entities"`
}

type entityItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// entitySchema is the JSON schema passed to Ollama's format parameter.
var entitySchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "entities": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "type": {"type": "string", "enum": ["topic", "concept"]}
        },
        "required": ["name", "type"]
      }
    }
  },
  "required": ["entities"]
}`)

func buildEntityPrompt(req ExtractionRequest) string {
	return fmt.Sprintf(`Extract topics and concepts from the following video content.
A "topic" is a broad subject area (e.g. "machine learning", "quantitative trading").
A "concept" is a specific idea, technique, or term (e.g. "market making", "gradient descent").
Return up to 20 entities. Prefer specific, reusable names. Avoid generic terms like "introduction" or "overview".

Title: %s
Description: %s
Content:
%s

Respond with JSON only: {"entities": [{"name": "...", "type": "topic|concept"}, ...]}`,
		req.Title, truncate(req.Description, 500), req.ContentText)
}

// truncate shortens s to at most n characters, appending "..." if cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// normaliseKey converts a display name to a stable ArangoDB-safe key.
// Rules: lowercase → collapse non-alphanumeric runs to "_" → trim leading/trailing "_".
func normaliseKey(name string) string {
	key := strings.ToLower(name)
	key = nonAlphanumRe.ReplaceAllString(key, "_")
	key = strings.Trim(key, "_")
	if len(key) > 240 {
		key = key[:240]
	}
	return key
}
