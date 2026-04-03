package prompt

import "fmt"

func PromptSingleSummary(title, text string) string {
	return fmt.Sprintf(`Summarize the following text in two concise paragraphs.

Video title: %s

Transcript:
%s

Respond with JSON in this exact format:
{"summary": "your summary here"}`, title, text)
}
