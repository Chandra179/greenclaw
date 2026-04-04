package youtube

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

// Options configures yt-dlp authentication and rate-limiting behaviour.
type Options struct {
	// CookiesFromBrowser passes --cookies-from-browser to yt-dlp (e.g. "chrome", "firefox").
	// Takes precedence over CookiesFile when both are set.
	CookiesFromBrowser string
	// CookiesFile is a path to a Netscape-format cookies.txt exported from a browser.
	CookiesFile string
	// SleepInterval is the minimum seconds to wait between requests (0 = no sleep).
	SleepInterval int
	// MaxSleepInterval is the maximum seconds for the randomised sleep window.
	// Ignored when SleepInterval is 0.
	MaxSleepInterval int
	// PlayerClients overrides the default player client fallback order.
	// Each entry is tried in order (e.g. ["web", "mweb", "android"]).
	// If empty, a sensible default list is used.
	PlayerClients []string
	// POToken is a YouTube Proof of Origin token for bypassing bot checks.
	POToken string
	// JSRuntime overrides the JS runtime yt-dlp uses (e.g. "nodejs", "deno").
	// If empty, yt-dlp picks automatically.
	JSRuntime string
	// UserAgent overrides the HTTP User-Agent header.
	UserAgent string
}

// strategy represents a single yt-dlp invocation attempt with specific extractor args.
type strategy struct {
	name       string
	clients    string // player_client value
	skipSteps  string // player_skip value (e.g. "webpage" to avoid 429 on initial page)
	extraFlags []string
}

// DownloadAudio downloads a low-bitrate audio track suitable for transcription.
// It tries multiple player-client strategies as a fallback mechanism to bypass
// bot detection and format restrictions.
func (c *Youtube) DownloadAudio(ctx context.Context, videoID, outputDir string, opts Options) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	if existing, err := findDownloadedAudio(outputDir, videoID); err == nil {
		log.Printf("audio already exists for %s, skipping download", videoID)
		return existing, nil
	}

	strategies := buildStrategies(opts)

	var lastErr error
	for _, s := range strategies {
		log.Printf("[ytdl] trying strategy %q for %s", s.name, videoID)
		path, err := tryDownload(ctx, videoID, outputDir, opts, s)
		if err == nil {
			return path, nil
		}
		lastErr = err
		log.Printf("[ytdl] strategy %q failed for %s: %v", s.name, videoID, summarizeError(err))
	}
	return "", fmt.Errorf("all yt-dlp strategies exhausted for %s: %w", videoID, lastErr)
}

func buildStrategies(opts Options) []strategy {
	clients := opts.PlayerClients
	if len(clients) == 0 {
		// Default fallback order: web (needs JS runtime + cookies for age-gated),
		// mweb (mobile web, sometimes less restrictive), android_vr (no PO token needed),
		// default (let yt-dlp decide).
		clients = []string{"web", "mweb", "android_vr", "default"}
	}

	var strategies []strategy
	for _, c := range clients {
		s := strategy{
			name:    c,
			clients: c,
		}
		// "default" means don't pass --extractor-args for player_client at all.
		if c == "default" {
			s.clients = ""
		}
		strategies = append(strategies, s)
	}

	// Final fallback: skip the webpage download entirely (avoids 429 on initial page load)
	// and let yt-dlp use its innate data API path.
	strategies = append(strategies, strategy{
		name:      "no-webpage",
		clients:   "",
		skipSteps: "webpage",
	})

	return strategies
}

func tryDownload(ctx context.Context, videoID, outputDir string, opts Options, s strategy) (string, error) {
	outTmpl := filepath.Join(outputDir, videoID+".%(ext)s")
	videoURL := "https://www.youtube.com/watch?v=" + videoID

	// "ba[abr<=64]/wa/ba": best audio ≤64kbps (speech), fallback worst audio, fallback best audio.
	args := []string{
		"--no-playlist",
		"-f", "ba[abr<=64]/wa/ba",
		"--no-overwrites",
		"--extract-audio",
		"--keep-video",
		"-o", outTmpl,
	}

	// Build extractor args
	var extractorParts []string
	if s.clients != "" {
		extractorParts = append(extractorParts, "player_client="+s.clients)
	}
	if s.skipSteps != "" {
		extractorParts = append(extractorParts, "player_skip="+s.skipSteps)
	}
	if opts.POToken != "" {
		extractorParts = append(extractorParts, "po_token="+opts.POToken)
	}
	if len(extractorParts) > 0 {
		args = append(args, "--extractor-args", "youtube:"+strings.Join(extractorParts, ";"))
	}

	// JS runtime
	if opts.JSRuntime != "" {
		args = append(args, "--js-runtimes", opts.JSRuntime)
	} else {
		// Prefer nodejs if available, since deno may not be installed
		args = append(args, "--js-runtimes", "node,deno,bun")
	}

	// Authentication
	switch {
	case opts.CookiesFromBrowser != "":
		args = append(args, "--cookies-from-browser", opts.CookiesFromBrowser)
	case opts.CookiesFile != "":
		if isValidCookiesFile(opts.CookiesFile) {
			args = append(args, "--cookies", opts.CookiesFile)
		}
	}

	// Rate limiting
	if opts.SleepInterval > 0 {
		args = append(args,
			"--sleep-interval", fmt.Sprintf("%d", opts.SleepInterval),
			"--max-sleep-interval", fmt.Sprintf("%d", max(opts.SleepInterval, opts.MaxSleepInterval)),
		)
	}

	// User agent
	if opts.UserAgent != "" {
		args = append(args, "--user-agent", opts.UserAgent)
	}

	// Extra per-strategy flags
	args = append(args, s.extraFlags...)

	args = append(args, "--", videoURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp failed: %w\n%s", err, out)
	}
	log.Printf("yt-dlp download completed for %s in %s (strategy: %s)", videoID, time.Since(start), s.name)

	return findDownloadedAudio(outputDir, videoID)
}

// summarizeError returns a short single-line summary of the error for logging.
func summarizeError(err error) string {
	s := err.Error()
	// Truncate verbose yt-dlp output to first meaningful line
	if idx := strings.Index(s, "\n"); idx > 0 {
		first := s[:idx]
		if len(first) > 120 {
			first = first[:120] + "..."
		}
		return first
	}
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

// CheckYTDLP verifies that yt-dlp is available in PATH.
func CheckYTDLP() error {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return fmt.Errorf("yt-dlp not found in PATH")
	}
	return nil
}

// isValidCookiesFile checks that the file exists and starts with the Netscape cookie header.
func isValidCookiesFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	if n == 0 {
		return false
	}
	return strings.Contains(string(buf[:n]), "# Netscape HTTP Cookie File") ||
		strings.Contains(string(buf[:n]), "# HTTP Cookie File")
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
