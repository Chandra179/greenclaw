package transcriber

import (
	"testing"
)

func TestParseWhisperOutput_JSON(t *testing.T) {
	input := []byte(`{
		"text": "Hello world, this is a test.",
		"language": "en",
		"duration": 5.2,
		"segments": [
			{"start": 0.0, "end": 2.5, "text": "Hello world,"},
			{"start": 2.5, "end": 5.2, "text": " this is a test."}
		]
	}`)

	result, err := parseWhisperOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello world, this is a test." {
		t.Errorf("text = %q, want %q", result.Text, "Hello world, this is a test.")
	}
	if result.Language != "en" {
		t.Errorf("language = %q, want %q", result.Language, "en")
	}
	if result.Duration != 5.2 {
		t.Errorf("duration = %f, want %f", result.Duration, 5.2)
	}
}

func TestParseWhisperOutput_SegmentsFallback(t *testing.T) {
	input := []byte(`{
		"text": "",
		"language": "en",
		"duration": 3.0,
		"segments": [
			{"start": 0.0, "end": 1.5, "text": "Hello"},
			{"start": 1.5, "end": 3.0, "text": "world"}
		]
	}`)

	result, err := parseWhisperOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello world" {
		t.Errorf("text = %q, want %q", result.Text, "Hello world")
	}
}

func TestParseWhisperOutput_PlainText(t *testing.T) {
	input := []byte("This is plain text output from whisper\n")

	result, err := parseWhisperOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "This is plain text output from whisper" {
		t.Errorf("text = %q, want %q", result.Text, "This is plain text output from whisper")
	}
}

func TestParseWhisperOutput_EmptyOutput(t *testing.T) {
	_, err := parseWhisperOutput([]byte(""))
	if err == nil {
		t.Error("expected error for empty output, got nil")
	}
}

func TestNewWhisperTranscriber(t *testing.T) {
	wt := NewWhisperTranscriber("/models/whisper")
	if wt.modelDir != "/models/whisper" {
		t.Errorf("modelDir = %q, want %q", wt.modelDir, "/models/whisper")
	}
}
