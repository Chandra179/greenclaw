package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port               int               `yaml:"port"`
	HTTPConcurrency    int               `yaml:"http_concurrency"`
	BrowserConcurrency int               `yaml:"browser_concurrency"`
	Timeout            time.Duration     `yaml:"timeout"`
	RetryAttempts      int               `yaml:"retry_attempts"`
	RecycleAfter       int               `yaml:"recycle_after"`
	YouTube            YouTubeConfig     `yaml:"youtube"`
	Transcriber        TranscriberConfig `yaml:"transcriber"`
}

// YouTubeConfig holds YouTube-specific extraction settings.
type YouTubeConfig struct {
	ExtractTranscripts bool     `yaml:"extract_transcripts"`
	TranscriptLangs    []string `yaml:"transcript_langs"`
	DownloadAudio      bool     `yaml:"download_audio"`
	AudioOutputDir     string   `yaml:"audio_output_dir"`
	SubtitleFormats    []string `yaml:"subtitle_formats"`
	SubtitleOutputDir  string   `yaml:"subtitle_output_dir"`
	TranscribeAudio    bool     `yaml:"transcribe_audio"`
	// CookiesFromBrowser passes --cookies-from-browser to yt-dlp (e.g. "chrome", "firefox").
	// Takes precedence over CookiesFile when both are set.
	CookiesFromBrowser string `yaml:"cookies_from_browser"`
	// CookiesFile is a path to a Netscape-format cookies.txt exported from a browser.
	CookiesFile string `yaml:"cookies_file"`
	// SleepInterval is the minimum seconds yt-dlp waits between requests (0 = disabled).
	SleepInterval int `yaml:"sleep_interval"`
	// MaxSleepInterval is the upper bound of the randomised sleep window.
	MaxSleepInterval int `yaml:"max_sleep_interval"`
	// PlayerClients overrides the player client fallback order (e.g. ["web", "mweb"]).
	// If empty, defaults to ["web", "mweb", "android_vr", "default"].
	PlayerClients []string `yaml:"player_clients"`
	// POToken is a YouTube Proof of Origin token for bypassing bot detection.
	POToken string `yaml:"po_token"`
	// JSRuntime overrides the JS runtime for yt-dlp (e.g. "nodejs", "deno").
	JSRuntime string `yaml:"js_runtime"`
	// UserAgent overrides the HTTP User-Agent header sent by yt-dlp.
	UserAgent string `yaml:"user_agent"`
}

// TranscriberConfig holds speech-to-text transcription settings.
type TranscriberConfig struct {
	Endpoint string `yaml:"endpoint"` // URL of remote whisper HTTP service
	Timeout  string `yaml:"timeout"`  // Transcription timeout (e.g. "5m")
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
			SubtitleFormats:    []string{"srt"},
			SubtitleOutputDir:  "downloads/subtitles",
			TranscribeAudio:    false,
		},
		Transcriber: TranscriberConfig{
			Timeout:  "5m",
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
