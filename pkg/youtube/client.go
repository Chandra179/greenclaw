package youtube

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	ytlib "github.com/kkdai/youtube/v2"
)

type Client interface {
	GetVideoMetadata(ctx context.Context, videoID string) (*VideoMetadata, *ytlib.Video, error)
	GetPlaylistItems(ctx context.Context, playlistID string) ([]PlaylistItem, error)
}

// Client wraps the kkdai/youtube library.
type Youtube struct {
	yt *ytlib.Client
}

// New creates a YouTube client using the given HTTP client.
func New(httpClient *http.Client) *Youtube {
	return &Youtube{yt: &ytlib.Client{HTTPClient: httpClient}}
}

// GetVideoMetadata fetches video metadata.
func (c *Youtube) GetVideoMetadata(ctx context.Context, videoID string) (*VideoMetadata, *ytlib.Video, error) {
	video, err := c.yt.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting video metadata: %w", err)
	}

	meta := &VideoMetadata{
		VideoID:     videoID,
		Title:       video.Title,
		Description: video.Description,
		Duration:    video.Duration.String(),
		ViewCount:   video.Views,
		ChannelName: video.Author,
		ChannelID:   video.ChannelID,
	}

	if video.PublishDate.After(time.Time{}) {
		meta.UploadDate = video.PublishDate.Format("2006-01-02")
	}

	for _, cap := range video.CaptionTracks {
		meta.Captions = append(meta.Captions, CaptionTrack{
			LanguageCode: cap.LanguageCode,
		})
	}

	return meta, video, nil
}

// GetPlaylistItems fetches all videos in a playlist.
func (c *Youtube) GetPlaylistItems(ctx context.Context, playlistID string) ([]PlaylistItem, error) {
	playlist, err := c.yt.GetPlaylistContext(ctx, playlistID)
	if err != nil {
		return nil, fmt.Errorf("getting playlist: %w", err)
	}

	items := make([]PlaylistItem, 0, len(playlist.Videos))
	for i, v := range playlist.Videos {
		items = append(items, PlaylistItem{VideoID: v.ID, Title: v.Title, Index: i})
	}
	return items, nil
}

// GetTranscript fetches a single caption track by language code.
func (c *Youtube) GetTranscript(ctx context.Context, video *ytlib.Video, langCode string) (*CaptionTrack, []TimedEntry, error) {
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

func (c *Youtube) fetchCaptionTrack(ctx context.Context, baseURL string) ([]TimedEntry, error) {
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
