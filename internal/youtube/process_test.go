package youtube

import (
	"context"
	"testing"

	"greenclaw/internal/router"
	"greenclaw/internal/store"
)

func TestProcessChannel(t *testing.T) {
	result, err := processChannel(context.Background(), "https://www.youtube.com/channel/UC123", "UC123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != store.ContentYouTubeChannel {
		t.Errorf("content type = %v, want %v", result.ContentType, store.ContentYouTubeChannel)
	}
	if result.Title != "Channel: UC123" {
		t.Errorf("title = %q, want %q", result.Title, "Channel: UC123")
	}
}

func TestProcessRouting(t *testing.T) {
	result, err := Process(context.Background(), New(nil), PipelineConfig{}, nil, nil, "https://www.youtube.com/channel/UC123", router.YouTubeChannel, "UC123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != store.ContentYouTubeChannel {
		t.Errorf("content type = %v, want %v", result.ContentType, store.ContentYouTubeChannel)
	}
}

func TestPipelineConfigDefaults(t *testing.T) {
	cfg := PipelineConfig{}
	if cfg.ExtractTranscripts {
		t.Error("ExtractTranscripts should default to false")
	}
	if cfg.DownloadAudio {
		t.Error("DownloadAudio should default to false")
	}
	if cfg.ExportSubtitles {
		t.Error("ExportSubtitles should default to false")
	}
	if cfg.TranscribeAudio {
		t.Error("TranscribeAudio should default to false")
	}
}

func TestPipelineConfigAllStages(t *testing.T) {
	cfg := PipelineConfig{
		ExtractTranscripts: true,
		TranscriptLangs:    []string{"en", "es"},
		DownloadAudio:      true,
		AudioOutputDir:     "/tmp/audio",
		ExportSubtitles:    true,
		SubtitleFormats:    []string{"srt", "vtt"},
		SubtitleOutputDir:  "/tmp/subs",
		TranscribeAudio:    true,
	}

	if !cfg.ExtractTranscripts {
		t.Error("ExtractTranscripts should be true")
	}
	if !cfg.TranscribeAudio {
		t.Error("TranscribeAudio should be true")
	}
	if len(cfg.TranscriptLangs) != 2 {
		t.Errorf("TranscriptLangs length = %d, want 2", len(cfg.TranscriptLangs))
	}
}

func TestPipelineTranscriptsOnly(t *testing.T) {
	cfg := PipelineConfig{
		ExtractTranscripts: true,
	}
	if !cfg.ExtractTranscripts {
		t.Error("ExtractTranscripts should be true")
	}
	if cfg.DownloadAudio {
		t.Error("DownloadAudio should be false")
	}
	if cfg.ExportSubtitles {
		t.Error("ExportSubtitles should be false")
	}
}

func TestPipelineAudioOnly(t *testing.T) {
	cfg := PipelineConfig{
		DownloadAudio:  true,
		AudioOutputDir: "/tmp/audio",
	}
	if cfg.ExtractTranscripts {
		t.Error("ExtractTranscripts should be false")
	}
	if !cfg.DownloadAudio {
		t.Error("DownloadAudio should be true")
	}
	if cfg.AudioOutputDir != "/tmp/audio" {
		t.Errorf("AudioOutputDir = %q, want %q", cfg.AudioOutputDir, "/tmp/audio")
	}
}
