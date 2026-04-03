package prompt

import (
	"fmt"
	"strings"
)

func PromptTakeaways(title, chunk string) string {
	return fmt.Sprintf(`Extract up to 5 key points from this portion of a YouTube video transcript titled "%s".

Transcript:
%s

Respond with JSON: {"key_points": ["point 1", "point 2"]}`, title, chunk)
}

func promptTakeawaysReduce(title string, points []string) string {
	var sb strings.Builder
	for _, p := range points {
		sb.WriteString("- ")
		sb.WriteString(p)
		sb.WriteByte('\n')
	}
	return fmt.Sprintf(`Below are key points extracted from different portions of a YouTube video titled "%s".

%s
Consolidate into up to 10 final takeaways ordered by importance. Remove duplicates.

Respond with JSON in this exact format:
{"takeaways": [{"text": "takeaway 1"}, {"text": "takeaway 2"}]}`, title, sb.String())
}
