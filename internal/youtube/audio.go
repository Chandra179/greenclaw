package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	ytlib "github.com/kkdai/youtube/v2"
)

// DownloadAudio downloads a low-bitrate audio track suitable for transcription.
func (c *Client) DownloadAudio(ctx context.Context, video *ytlib.Video, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	outTmpl := filepath.Join(outputDir, video.ID+".%(ext)s")
	videoURL := "https://www.youtube.com/watch?v=" + video.ID

	// OPTIMIZATION:
	// "ba[abr<=64]/wa/ba" means:
	// 1. Try to get Best Audio with a bitrate <= 64kbps (perfect for speech)
	// 2. Fallback to Worst Audio (wa)
	// 3. Fallback to Best Audio (ba) if the others fail
	args := []string{
		"--no-playlist",
		"-f", "ba[abr<=64]/wa/ba",
		"--no-overwrites",
		// Use --extract-audio just to ensure we don't accidentally get video streams
		"--extract-audio",
		// Keep the original format (m4a/webm) to save CPU, don't re-encode!
		"--keep-video",
		"-o", outTmpl,
		"--", videoURL,
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stderr = os.Stderr // Pipe stderr to console so you can see yt-dlp download progress

	if out, err := cmd.Output(); err != nil {
		return "", fmt.Errorf("yt-dlp failed: %w\n%s", err, out)
	}

	dest, err := findDownloadedAudio(outputDir, video.ID)
	if err != nil {
		return "", err
	}
	return dest, nil
}

// findDownloadedAudio locates the audio file written by yt-dlp.
func findDownloadedAudio(dir, videoID string) (string, error) {
	// yt-dlp usually leaves these extensions when extracting native audio
	for _, ext := range []string{".m4a", ".webm", ".opus", ".ogg", ".mp3"} {
		p := filepath.Join(dir, videoID+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Fallback: glob for any file matching the video ID
	matches, _ := filepath.Glob(filepath.Join(dir, videoID+".*"))
	for _, m := range matches {
		lower := strings.ToLower(m)
		if strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".ytdl") {
			continue
		}
		return m, nil
	}
	return "", fmt.Errorf("audio file not found after yt-dlp completed for %s", videoID)
}

func CheckYTDLP() error {
	_, err := exec.LookPath("yt-dlp")
	if err != nil {
		return fmt.Errorf("yt-dlp not found in PATH")
	}
	return nil
}
