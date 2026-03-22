package youtube

import (
	"context"

	"greenclaw/pkg/ytdl"

	ytlib "github.com/kkdai/youtube/v2"
)

// DownloadAudio downloads a low-bitrate audio track suitable for transcription.
func (c *Client) DownloadAudio(ctx context.Context, video *ytlib.Video, outputDir string) (string, error) {
	return ytdl.DownloadAudio(ctx, video.ID, outputDir)
}

// CheckYTDLP verifies that yt-dlp is available in PATH.
func CheckYTDLP() error {
	return ytdl.CheckYTDLP()
}
