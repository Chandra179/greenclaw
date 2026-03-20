package youtube

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ytlib "github.com/kkdai/youtube/v2"
)

// DownloadAudio selects the best audio-only stream, downloads it to outputDir,
// and returns the file path. Prefers opus/webm, falls back to m4a/mp4.
func (c *Client) DownloadAudio(ctx context.Context, video *ytlib.Video, outputDir string) (string, error) {
	formats := video.Formats.Type("audio")
	if len(formats) == 0 {
		return "", fmt.Errorf("no audio-only streams available for video %s", video.ID)
	}

	// Prefer opus/webm, then m4a
	var selected *ytlib.Format
	for i := range formats {
		f := &formats[i]
		if strings.Contains(f.MimeType, "opus") || strings.Contains(f.MimeType, "webm") {
			if selected == nil || f.Bitrate > selected.Bitrate {
				selected = f
			}
		}
	}
	if selected == nil {
		// Fallback: pick highest bitrate audio
		best := &formats[0]
		for i := 1; i < len(formats); i++ {
			if formats[i].Bitrate > best.Bitrate {
				best = &formats[i]
			}
		}
		selected = best
	}

	stream, _, err := c.yt.GetStreamContext(ctx, video, selected)
	if err != nil {
		return "", fmt.Errorf("getting audio stream: %w", err)
	}
	defer stream.Close()

	ext := "webm"
	if strings.Contains(selected.MimeType, "mp4") || strings.Contains(selected.MimeType, "m4a") {
		ext = "m4a"
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	filename := fmt.Sprintf("%s.%s", video.ID, ext)
	dest := filepath.Join(outputDir, filename)

	f, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("creating audio file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, stream); err != nil {
		return "", fmt.Errorf("writing audio file: %w", err)
	}

	return dest, nil
}
