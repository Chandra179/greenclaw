package transcriber

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// WhisperTranscriber wraps the faster-whisper CLI as a subprocess.
type WhisperTranscriber struct {
	modelDir string
}

// NewWhisperTranscriber creates a transcriber that uses faster-whisper.
// modelDir is the directory where whisper model files are stored.
func NewWhisperTranscriber(modelDir string) *WhisperTranscriber {
	return &WhisperTranscriber{modelDir: modelDir}
}

// Transcribe runs faster-whisper on the given audio file and returns the result.
func (w *WhisperTranscriber) Transcribe(ctx context.Context, audioPath string, opts Options) (*Result, error) {
	model := opts.Model
	if model == "" {
		model = "base"
	}

	// Use a temp directory for JSON output — faster-whisper writes the JSON
	// to a file, while stdout only contains timestamped progress lines.
	tmpDir, err := os.MkdirTemp("", "whisper-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	args := []string{
		audioPath,
		"--model", model,
		"--output_format", "json",
		"--output_dir", tmpDir,
	}

	if opts.ModelDir != "" {
		args = append(args, "--model_dir", opts.ModelDir)
	} else if w.modelDir != "" {
		args = append(args, "--model_dir", w.modelDir)
	}

	if opts.Language != "" {
		args = append(args, "--language", opts.Language)
	}

	if opts.Task == "translate" {
		args = append(args, "--task", "translate")
	}

	cmd := exec.CommandContext(ctx, "faster-whisper", args...)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("faster-whisper failed: %w\n%s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("faster-whisper failed: %w", err)
	}

	// faster-whisper writes <basename>.json in the output directory
	base := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	jsonPath := filepath.Join(tmpDir, base+".json")

	out, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("reading whisper JSON output: %w", err)
	}

	return parseWhisperOutput(out)
}

// whisperJSON represents the JSON output structure from faster-whisper.
type whisperJSON struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Duration float64          `json:"duration"`
	Segments []whisperSegment `json:"segments"`
}

type whisperSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// timestampRe matches whisper timestamp lines like "[00:00.000 --> 00:01.600]"
var timestampRe = regexp.MustCompile(`\[\d{2}:\d{2}\.\d{3}\s*-->\s*\d{2}:\d{2}\.\d{3}\]\s*`)

// stripTimestamps removes VTT-style timestamp prefixes from whisper plain text output.
func stripTimestamps(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		line = timestampRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, " ")
}

func parseWhisperOutput(data []byte) (*Result, error) {
	var wj whisperJSON
	if err := json.Unmarshal(data, &wj); err != nil {
		// Fallback: treat output as plain text, stripping any timestamps
		text := stripTimestamps(string(data))
		if text == "" {
			return nil, fmt.Errorf("faster-whisper returned empty output")
		}
		return &Result{Text: text}, nil
	}

	text := wj.Text
	if text == "" && len(wj.Segments) > 0 {
		var parts []string
		for _, seg := range wj.Segments {
			parts = append(parts, strings.TrimSpace(seg.Text))
		}
		text = strings.Join(parts, " ")
	}

	return &Result{
		Text:     strings.TrimSpace(text),
		Language: wj.Language,
		Duration: wj.Duration,
	}, nil
}

// CheckWhisper returns an error if faster-whisper is not installed or not found in PATH.
func CheckWhisper() error {
	_, err := exec.LookPath("faster-whisper")
	if err != nil {
		return fmt.Errorf("faster-whisper not found in PATH: install it via 'pip install faster-whisper'")
	}
	return nil
}
