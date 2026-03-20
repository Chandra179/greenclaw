package transcriber

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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

	args := []string{
		audioPath,
		"--model", model,
		"--output_format", "json",
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

	args = append(args, "--output_dir", "-")

	cmd := exec.CommandContext(ctx, "faster-whisper", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("faster-whisper failed: %w\n%s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("faster-whisper failed: %w", err)
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

func parseWhisperOutput(data []byte) (*Result, error) {
	var wj whisperJSON
	if err := json.Unmarshal(data, &wj); err != nil {
		// Fallback: treat output as plain text
		text := strings.TrimSpace(string(data))
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
