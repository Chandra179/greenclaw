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

// DownloadAudio downloads the audio track of a YouTube video using yt-dlp.
// When ffmpeg is available, it extracts and converts to opus. Otherwise it
// downloads the best audio-only stream in its native format (typically webm).
// The yt-dlp binary must be installed and available in PATH.
func (c *Client) DownloadAudio(ctx context.Context, video *ytlib.Video, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	outTmpl := filepath.Join(outputDir, video.ID+".%(ext)s")
	videoURL := "https://www.youtube.com/watch?v=" + video.ID

	args := []string{"--no-playlist", "-f", "bestaudio", "--no-overwrites", "-o", outTmpl}

	// Extract and convert to opus when ffmpeg is available.
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		args = append(args, "-x", "--audio-format", "opus")
	}

	args = append(args, "--", videoURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stderr = os.Stderr
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
	for _, ext := range []string{".opus", ".webm", ".m4a", ".ogg", ".mp3"} {
		p := filepath.Join(dir, videoID+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Fallback: glob for any file matching the video ID
	matches, _ := filepath.Glob(filepath.Join(dir, videoID+".*"))
	for _, m := range matches {
		lower := strings.ToLower(m)
		if strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".json") {
			continue
		}
		return m, nil
	}
	return "", fmt.Errorf("audio file not found after yt-dlp completed for %s", videoID)
}

// CheckYTDLP returns an error if yt-dlp is not installed or not found in PATH.
func CheckYTDLP() error {
	_, err := exec.LookPath("yt-dlp")
	if err != nil {
		return fmt.Errorf("yt-dlp not found in PATH: install it via 'pip install yt-dlp' or 'brew install yt-dlp'")
	}
	return nil
}
