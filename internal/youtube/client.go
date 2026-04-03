package youtube

import (
	"context"
	"net/http"

	"greenclaw/internal/store"
	pkgyt "greenclaw/pkg/youtube"

	ytlib "github.com/kkdai/youtube/v2"
)

// Client wraps pkg/youtube.Client and converts types for internal use.
type Client struct {
	pkg *pkgyt.Client
}

// New creates a YouTube client using the given HTTP client.
func New(httpClient *http.Client) *Client {
	return &Client{pkg: pkgyt.New(httpClient)}
}

// GetVideoMetadata fetches video metadata and returns a populated YouTubeData struct.
func (c *Client) GetVideoMetadata(ctx context.Context, videoID string) (*store.YouTubeData, *ytlib.Video, error) {
	meta, video, err := c.pkg.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return nil, nil, err
	}

	data := &store.YouTubeData{
		VideoID:     meta.VideoID,
		Duration:    meta.Duration,
		ViewCount:   meta.ViewCount,
		UploadDate:  meta.UploadDate,
		ChannelName: meta.ChannelName,
		ChannelID:   meta.ChannelID,
	}
	for _, cap := range meta.Captions {
		data.Captions = append(data.Captions, store.CaptionTrack{
			LanguageCode: cap.LanguageCode,
		})
	}

	return data, video, nil
}

// GetPlaylistItems fetches all videos in a playlist.
func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]store.PlaylistItem, error) {
	items, err := c.pkg.GetPlaylistItems(ctx, playlistID)
	if err != nil {
		return nil, err
	}

	out := make([]store.PlaylistItem, len(items))
	for i, item := range items {
		out[i] = store.PlaylistItem{
			VideoID: item.VideoID,
			Title:   item.Title,
			Index:   item.Index,
		}
	}
	return out, nil
}
