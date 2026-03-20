package youtube

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	ytlib "github.com/kkdai/youtube/v2"
)

// SubtitleFormat represents an output subtitle format.
type SubtitleFormat string

const (
	FormatSRT  SubtitleFormat = "srt"
	FormatVTT  SubtitleFormat = "vtt"
	FormatTTML SubtitleFormat = "ttml"
)

// ParseSubtitleFormat converts a string to a SubtitleFormat, returning an error
// for unknown formats.
func ParseSubtitleFormat(s string) (SubtitleFormat, error) {
	switch strings.ToLower(s) {
	case "srt":
		return FormatSRT, nil
	case "vtt", "webvtt":
		return FormatVTT, nil
	case "ttml":
		return FormatTTML, nil
	default:
		return "", fmt.Errorf("unknown subtitle format: %s", s)
	}
}

// ExportSubtitles fetches a caption track, converts to the specified format,
// writes to outputDir, and returns the file path.
func (c *Client) ExportSubtitles(ctx context.Context, video *ytlib.Video, langCode string, format SubtitleFormat, outputDir string) (string, error) {
	entries, err := c.GetTranscriptEntries(ctx, video, langCode)
	if err != nil {
		return "", err
	}

	var content string
	switch format {
	case FormatSRT:
		content = toSRT(entries)
	case FormatVTT:
		content = toVTT(entries)
	case FormatTTML:
		content = toTTML(entries)
	default:
		return "", fmt.Errorf("unsupported subtitle format: %s", format)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	filename := fmt.Sprintf("%s.%s.%s", video.ID, langCode, format)
	dest := filepath.Join(outputDir, filename)

	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing subtitle file: %w", err)
	}

	return dest, nil
}

// toSRT converts timed entries to SubRip (.srt) format.
func toSRT(entries []TimedEntry) string {
	var b strings.Builder
	for i, e := range entries {
		fmt.Fprintf(&b, "%d\n", i+1)
		fmt.Fprintf(&b, "%s --> %s\n", formatSRTTime(e.Start), formatSRTTime(e.Start+e.Dur))
		fmt.Fprintf(&b, "%s\n\n", e.Text)
	}
	return b.String()
}

// toVTT converts timed entries to WebVTT (.vtt) format.
func toVTT(entries []TimedEntry) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "%s --> %s\n", formatVTTTime(e.Start), formatVTTTime(e.Start+e.Dur))
		fmt.Fprintf(&b, "%s\n\n", e.Text)
	}
	return b.String()
}

// toTTML converts timed entries to TTML format.
func toTTML(entries []TimedEntry) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString("\n")
	b.WriteString(`<tt xmlns="http://www.w3.org/ns/ttml">`)
	b.WriteString("\n  <body>\n    <div>\n")
	for _, e := range entries {
		fmt.Fprintf(&b, `      <p begin="%s" end="%s">%s</p>`+"\n",
			formatTTMLTime(e.Start), formatTTMLTime(e.Start+e.Dur), e.Text)
	}
	b.WriteString("    </div>\n  </body>\n</tt>\n")
	return b.String()
}

// formatSRTTime formats seconds as HH:MM:SS,mmm.
func formatSRTTime(seconds float64) string {
	h, m, s, ms := splitTime(seconds)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// formatVTTTime formats seconds as HH:MM:SS.mmm.
func formatVTTTime(seconds float64) string {
	h, m, s, ms := splitTime(seconds)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// formatTTMLTime formats seconds as HH:MM:SS.mmm.
func formatTTMLTime(seconds float64) string {
	return formatVTTTime(seconds)
}

func splitTime(seconds float64) (int, int, int, int) {
	total := int(math.Round(seconds * 1000))
	ms := total % 1000
	total /= 1000
	s := total % 60
	total /= 60
	m := total % 60
	h := total / 60
	return h, m, s, ms
}
