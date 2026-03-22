package youtube

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ytlib "github.com/kkdai/youtube/v2"
)

// VideoMetadata holds basic metadata for a YouTube video.
type VideoMetadata struct {
	VideoID     string
	Title       string
	Description string
	Duration    string
	ViewCount   int
	UploadDate  string
	ChannelName string
	ChannelID   string
	Captions    []CaptionTrack
}

// PlaylistItem represents a single video in a playlist.
type PlaylistItem struct {
	VideoID string
	Title   string
	Index   int
}

// Client wraps the kkdai/youtube library.
type Client struct {
	yt *ytlib.Client
}

// New creates a YouTube client using the given HTTP client.
func New(httpClient *http.Client) *Client {
	return &Client{yt: &ytlib.Client{HTTPClient: httpClient}}
}

// GetVideoMetadata fetches video metadata.
func (c *Client) GetVideoMetadata(ctx context.Context, videoID string) (*VideoMetadata, *ytlib.Video, error) {
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
func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]PlaylistItem, error) {
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
