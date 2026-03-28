package llm

import "encoding/json"

// SummaryResponse is the expected JSON structure for StyleSummary.
type SummaryResponse struct {
	Summary string `json:"summary"`
}

// TakeawaysResponse is the expected JSON structure for StyleTakeaways.
type TakeawaysResponse struct {
	Takeaways []Takeaway `json:"takeaways"`
}

// Takeaway is a single bullet point within a TakeawaysResponse.
type Takeaway struct {
	Text string `json:"text"`
}

// ollamaSchema returns the JSON schema for Ollama structured output.
// Returns nil for unknown styles, which falls back to plain "json" mode.
func ollamaSchema(style ProcessingStyle) json.RawMessage {
	switch style {
	case StyleSummary:
		return json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"]}`)
	case StyleTakeaways:
		return json.RawMessage(`{"type":"object","properties":{"takeaways":{"type":"array","items":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}},"required":["takeaways"]}`)
	}
	return nil
}
