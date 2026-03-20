package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port               int              `yaml:"port"`
	HTTPConcurrency    int              `yaml:"http_concurrency"`
	BrowserConcurrency int              `yaml:"browser_concurrency"`
	Timeout            time.Duration    `yaml:"timeout"`
	RetryAttempts      int              `yaml:"retry_attempts"`
	RecycleAfter       int              `yaml:"recycle_after"`
	YouTube            YouTubeConfig    `yaml:"youtube"`
	Transcriber        TranscriberConfig `yaml:"transcriber"`
}

// YouTubeConfig holds YouTube-specific extraction settings.
type YouTubeConfig struct {
	ExtractTranscripts bool     `yaml:"extract_transcripts"`
	TranscriptLangs    []string `yaml:"transcript_langs"`
	DownloadAudio      bool     `yaml:"download_audio"`
	AudioOutputDir     string   `yaml:"audio_output_dir"`
	ExportSubtitles    bool     `yaml:"export_subtitles"`
	SubtitleFormats    []string `yaml:"subtitle_formats"`
	SubtitleOutputDir  string   `yaml:"subtitle_output_dir"`
	TranscribeAudio    bool     `yaml:"transcribe_audio"`
}

// TranscriberConfig holds speech-to-text transcription settings.
type TranscriberConfig struct {
	Model    string `yaml:"model"`
	ModelDir string `yaml:"model_dir"`
	Language string `yaml:"language"`
}

func Default() Config {
	return Config{
		Port:               8080,
		HTTPConcurrency:    20,
		BrowserConcurrency: 5,
		Timeout:            30 * time.Second,
		RetryAttempts:      3,
		RecycleAfter:       100,
		YouTube: YouTubeConfig{
			ExtractTranscripts: true,
			TranscriptLangs:    nil,
			DownloadAudio:      false,
			AudioOutputDir:     "downloads/audio",
			ExportSubtitles:    false,
			SubtitleFormats:    []string{"srt"},
			SubtitleOutputDir:  "downloads/subtitles",
			TranscribeAudio:    false,
		},
		Transcriber: TranscriberConfig{
			Model:    "base",
			ModelDir: "/models/whisper",
			Language: "",
		},
	}
}

// Load reads config.yaml if it exists and overrides defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	return cfg, yaml.NewDecoder(f).Decode(&cfg)
}
