package youtube

import (
	"context"
	"net/http"

	ytlib "github.com/kkdai/youtube/v2"
)

type Client interface {
	GetVideoMetadata(ctx context.Context, videoID string) (*VideoMetadata, *ytlib.Video, error)
	GetPlaylistItems(ctx context.Context, playlistID string) ([]PlaylistItem, error)
	GetTranscript(ctx context.Context, video *ytlib.Video, langCode string) (*CaptionTrack, []TimedEntry, error)
	DownloadAudio(ctx context.Context, videoID, outputDir string, opts Options) (string, error)
}

// Client wraps the kkdai/youtube library.
type Youtube struct {
	yt *ytlib.Client
}

// New creates a YouTube client using the given HTTP client.
func New(httpClient *http.Client) *Youtube {
	return &Youtube{yt: &ytlib.Client{HTTPClient: httpClient}}
}

// ContentType represents the detected content type of a URL.
type YoutubeContentType string

// Content type constants for YouTube URLs.
const (
	YoutubeContentVideo    YoutubeContentType = "youtube_video"
	YoutubeContentPlaylist YoutubeContentType = "youtube_playlist"
	YoutubeContentChannel  YoutubeContentType = "youtube_channel"
)
