package llm

import (
	"context"
	"encoding/json"
)

type ChatRequest struct {
	Prompt string
	Schema json.RawMessage
	NumCtx int
}

type ChatResponse struct {
	JsonResponse json.RawMessage
}

type Client interface {
	Chat(ctx context.Context, req ChatRequest) ChatResponse
}
