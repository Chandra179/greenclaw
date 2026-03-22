package ytdl

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DownloadAudio downloads a low-bitrate audio track suitable for transcription.
// It calls yt-dlp and returns the path to the downloaded file.
func DownloadAudio(ctx context.Context, videoID, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	if existing, err := findDownloadedAudio(outputDir, videoID); err == nil {
		log.Printf("audio already exists for %s, skipping download", videoID)
		return existing, nil
	}

	outTmpl := filepath.Join(outputDir, videoID+".%(ext)s")
	videoURL := "https://www.youtube.com/watch?v=" + videoID

	// "ba[abr<=64]/wa/ba": best audio ≤64kbps (speech), fallback worst, fallback best.
	args := []string{
		"--no-playlist",
		"-f", "ba[abr<=64]/wa/ba",
		"--no-overwrites",
		"--extract-audio",
		"--keep-video",
		"-o", outTmpl,
		"--", videoURL,
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stderr = os.Stderr

	start := time.Now()
	if out, err := cmd.Output(); err != nil {
		return "", fmt.Errorf("yt-dlp failed: %w\n%s", err, out)
	}
	log.Printf("yt-dlp download completed for %s in %s", videoID, time.Since(start))

	return findDownloadedAudio(outputDir, videoID)
}

// CheckYTDLP verifies that yt-dlp is available in PATH.
func CheckYTDLP() error {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return fmt.Errorf("yt-dlp not found in PATH")
	}
	return nil
}

func findDownloadedAudio(dir, videoID string) (string, error) {
	for _, ext := range []string{".m4a", ".webm", ".opus", ".ogg", ".mp3"} {
		p := filepath.Join(dir, videoID+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

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
