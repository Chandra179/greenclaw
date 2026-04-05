package graph

import (
	"context"
	"encoding/json"
	"fmt"

	"greenclaw/pkg/llm"
)

var classifySchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "category": { "type": "string", "enum": ["economy", "technology"] }
  },
  "required": ["category"]
}`)

type classifyResponse struct {
	Category string `json:"category"`
}

// Classify asks the LLM to assign a category to a transcript.
// Only the first 2000 chars are sent — enough signal, cheap call.
func Classify(ctx context.Context, lc llmClient, transcript string, numCtx int) (string, error) {
	excerpt := transcript
	if len(excerpt) > 2000 {
		excerpt = excerpt[:2000]
	}

	prompt := fmt.Sprintf(`Classify the following video transcript into exactly one category.

Categories:
- economy: macroeconomics, finance, markets, trade, policy
- technology: software, hardware, AI, engineering, research

Transcript excerpt:
%s

Respond with JSON only.`, excerpt)

	resp, err := lc.Chat(ctx, llm.ChatRequest{
		Prompt: prompt,
		Schema: classifySchema,
		NumCtx: numCtx,
	})
	if err != nil {
		return "", fmt.Errorf("LLM classify: %w", err)
	}

	var cr classifyResponse
	if err := json.Unmarshal(resp.JsonResponse, &cr); err != nil {
		return "", fmt.Errorf("parse classify response: %w", err)
	}

	switch cr.Category {
	case "economy", "technology":
		return cr.Category, nil
	default:
		return "technology", nil
	}
}
