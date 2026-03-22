package llm

// SummaryResponse is the expected JSON structure for StyleSummary.
type SummaryResponse struct {
	Summary string `json:"summary" jsonschema:"description=A concise one or two paragraph summary of the video,minLength=10"`
}

// TakeawaysResponse is the expected JSON structure for StyleTakeaways.
type TakeawaysResponse struct {
	Takeaways []Takeaway `json:"takeaways" jsonschema:"description=Key takeaways ordered by importance,minItems=1,maxItems=10"`
}

// Takeaway is a single bullet point within a TakeawaysResponse.
type Takeaway struct {
	Text string `json:"text" jsonschema:"description=A single key takeaway,minLength=5"`
}
