package youtube

import (
	"context"
	"log"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"greenclaw/internal/router"
	"greenclaw/internal/store"
	"greenclaw/pkg/transcribe"

	ytlib "github.com/kkdai/youtube/v2"
)

// PipelineConfig holds all settings needed by YouTube processing.
type PipelineConfig struct {
	ExtractTranscripts bool
	TranscriptLangs    []string
	DownloadAudio      bool
	AudioOutputDir     string
	ExportSubtitles    bool
	SubtitleFormats    []string
	SubtitleOutputDir  string
	TranscribeAudio    bool
}

// Process handles a YouTube URL based on its type (video, playlist, channel).
func Process(ctx context.Context, client *Client, cfg PipelineConfig, t transcribe.Client, url string, ytType router.YouTubeURLType, id string) (*store.Result, error) {
	switch ytType {
	case router.YouTubePlaylist:
		return processPlaylist(ctx, client, url, id)
	case router.YouTubeChannel:
		return processChannel(ctx, url, id)
	default:
		return processVideo(ctx, client, cfg, t, url, id)
	}
}

func processVideo(ctx context.Context, client *Client, cfg PipelineConfig, t transcribe.Client, url, videoID string) (*store.Result, error) {
	// Stage 1: metadata (sequential — everything else depends on it)
	ytData, video, err := client.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return nil, err
	}

	result := &store.Result{
		URL:         url,
		ContentType: store.ContentYouTubeVideo,
		Title:       video.Title,
		Description: video.Description,
		Stage:       1,
		FetchedAt:   time.Now(),
	}

	// Stage 2: parallel extraction — each goroutine writes to its own variable
	var (
		captions    []store.CaptionTrack
		captionText string
		audioPath   string
	)

	g, gctx := errgroup.WithContext(ctx)

	if cfg.ExtractTranscripts {
		g.Go(func() error {
			captions, captionText = fetchTranscripts(gctx, client, cfg, video, videoID)
			return nil
		})
	}

	if cfg.DownloadAudio {
		g.Go(func() error {
			audioPath = downloadAudio(gctx, client, cfg, video, videoID)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Stage 3: merge results (sequential)
	if len(captions) > 0 {
		ytData.Captions = captions
	}
	if audioPath != "" {
		ytData.AudioPath = audioPath
		result.FilePath = audioPath
	}

	// Stage 4: transcribe audio (sequential, requires audio)
	var whisperText string
	if audioPath != "" && cfg.TranscribeAudio {
		whisperText = transcribeAudio(ctx, t, audioPath, videoID)
	}

	// Choose text priority: captions > whisper
	if captionText != "" {
		result.Text = captionText
	} else if whisperText != "" {
		result.Text = whisperText
	}

	result.RawData = ytData
	return result, nil
}

func fetchTranscripts(ctx context.Context, client *Client, cfg PipelineConfig, video *ytlib.Video, videoID string) ([]store.CaptionTrack, string) {
	var tracks []store.CaptionTrack
	var firstText string
	for _, lang := range cfg.TranscriptLangs {
		track, _, err := client.GetTranscript(ctx, video, lang)
		if err != nil {
			log.Printf("[youtube] transcript %s failed for %s: %v", lang, videoID, err)
			continue
		}
		tracks = append(tracks, *track)
		if firstText == "" {
			firstText = track.Text
		}
	}
	return tracks, firstText
}

func downloadAudio(ctx context.Context, client *Client, cfg PipelineConfig, video *ytlib.Video, videoID string) string {
	if err := CheckYTDLP(); err != nil {
		log.Printf("[youtube] %v", err)
		return ""
	}
	audioPath, err := client.DownloadAudio(ctx, video, cfg.AudioOutputDir)
	if err != nil {
		log.Printf("[youtube] audio download failed for %s: %v", videoID, err)
		return ""
	}
	return audioPath
}

func transcribeAudio(ctx context.Context, t transcribe.Client, audioPath, videoID string) string {
	if t == nil {
		log.Printf("[youtube] no transcriber configured, skipping %s", videoID)
		return ""
	}
	tr, err := t.Transcribe(ctx, audioPath)
	if err != nil {
		log.Printf("[youtube] transcription failed for %s: %v", videoID, err)
		return ""
	}
	return tr.Text
}

func processPlaylist(ctx context.Context, client *Client, url, playlistID string) (*store.Result, error) {
	items, err := client.GetPlaylistItems(ctx, playlistID)
	if err != nil {
		return nil, err
	}

	var titles []string
	for _, item := range items {
		titles = append(titles, item.Title)
	}

	ytData := &store.YouTubeData{
		PlaylistItems: items,
	}

	return &store.Result{
		URL:         url,
		ContentType: store.ContentYouTubePlaylist,
		Title:       "Playlist: " + playlistID,
		Description: strings.Join(titles, "; "),
		RawData:     ytData,
		Stage:       1,
		FetchedAt:   time.Now(),
	}, nil
}

func processChannel(_ context.Context, url, channelID string) (*store.Result, error) {
	ytData := &store.YouTubeData{
		ChannelID: channelID,
	}

	return &store.Result{
		URL:         url,
		ContentType: store.ContentYouTubeChannel,
		Title:       "Channel: " + channelID,
		RawData:     ytData,
		Stage:       1,
		FetchedAt:   time.Now(),
	}, nil
}
