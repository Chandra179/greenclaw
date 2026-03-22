package youtube

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"

	ytlib "github.com/kkdai/youtube/v2"
)

// CaptionTrack represents a single caption/subtitle track.
type CaptionTrack struct {
	LanguageCode string
	Text         string
}

// TimedEntry represents a single timed text entry from YouTube's caption XML.
type TimedEntry struct {
	Start float64 `xml:"start,attr"`
	Dur   float64 `xml:"dur,attr"`
	Text  string  `xml:",chardata"`
}

type timedTextXML struct {
	Entries []TimedEntry `xml:"text"`
}

// GetTranscript fetches a single caption track by language code.
func (c *Client) GetTranscript(ctx context.Context, video *ytlib.Video, langCode string) (*CaptionTrack, []TimedEntry, error) {
	for _, cap := range video.CaptionTracks {
		if cap.LanguageCode == langCode {
			entries, err := c.fetchCaptionTrack(ctx, cap.BaseURL)
			if err != nil {
				return nil, nil, err
			}
			track := &CaptionTrack{
				LanguageCode: cap.LanguageCode,
				Text:         entriesToPlainText(entries),
			}
			return track, entries, nil
		}
	}
	return nil, nil, fmt.Errorf("no caption track found for language: %s", langCode)
}

func (c *Client) fetchCaptionTrack(ctx context.Context, baseURL string) ([]TimedEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating caption request: %w", err)
	}

	httpClient := c.yt.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching caption track: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading caption data: %w", err)
	}

	return parseTimedTextXML(data)
}

func parseTimedTextXML(data []byte) ([]TimedEntry, error) {
	var tt timedTextXML
	if err := xml.Unmarshal(data, &tt); err != nil {
		return nil, fmt.Errorf("parsing timed text XML: %w", err)
	}
	for i := range tt.Entries {
		tt.Entries[i].Text = html.UnescapeString(tt.Entries[i].Text)
	}
	return tt.Entries, nil
}

func entriesToPlainText(entries []TimedEntry) string {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(strings.TrimSpace(e.Text))
	}
	return b.String()
}
