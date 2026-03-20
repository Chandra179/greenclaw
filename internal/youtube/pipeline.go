package youtube

import (
	"context"
	"log"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"greenclaw/internal/router"
	"greenclaw/internal/store"
	"greenclaw/internal/transcriber"

	ytlib "github.com/kkdai/youtube/v2"
)

// PipelineConfig holds all settings needed by the YouTube pipeline.
type PipelineConfig struct {
	ExtractTranscripts bool
	TranscriptLangs    []string
	DownloadAudio      bool
	AudioOutputDir     string
	ExportSubtitles    bool
	SubtitleFormats    []string
	SubtitleOutputDir  string
	TranscribeAudio    bool

	// Transcriber settings
	TranscriberModel    string
	TranscriberModelDir string
	TranscriberLanguage string
}

// Pipeline orchestrates YouTube extraction as a multi-stage pipeline.
type Pipeline struct {
	client *Client
	cfg    PipelineConfig
}

// NewPipeline creates a Pipeline with the given client and config.
func NewPipeline(client *Client, cfg PipelineConfig) *Pipeline {
	return &Pipeline{client: client, cfg: cfg}
}

// Process handles a YouTube URL based on its type (video, playlist, channel).
func (p *Pipeline) Process(ctx context.Context, url string, ytType router.YouTubeURLType, id string) (*store.Result, error) {
	switch ytType {
	case router.YouTubePlaylist:
		return p.processPlaylist(ctx, url, id)
	case router.YouTubeChannel:
		return p.processChannel(ctx, url, id)
	default:
		return p.processVideo(ctx, url, id)
	}
}

func (p *Pipeline) processVideo(ctx context.Context, url, videoID string) (*store.Result, error) {
	// Stage 1: metadata (sequential — everything else depends on it)
	ytData, video, err := p.client.GetVideoMetadata(ctx, videoID)
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
		subPaths    map[string]string
		audioPath   string
	)

	g, gctx := errgroup.WithContext(ctx)

	if p.cfg.ExtractTranscripts {
		g.Go(func() error {
			captions, captionText = p.fetchTranscripts(gctx, video, videoID)
			return nil
		})
	}

	if p.cfg.ExportSubtitles {
		g.Go(func() error {
			subPaths = p.exportSubtitles(gctx, video, videoID)
			return nil
		})
	}

	if p.cfg.DownloadAudio {
		g.Go(func() error {
			audioPath = p.downloadAudio(gctx, video, videoID)
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
	if len(subPaths) > 0 {
		ytData.SubtitlePaths = subPaths
	}
	if audioPath != "" {
		ytData.AudioPath = audioPath
		result.FilePath = audioPath
	}

	// Stage 4: transcribe audio (sequential, requires audio)
	var whisperText string
	if audioPath != "" && p.cfg.TranscribeAudio {
		whisperText = p.transcribeAudio(ctx, audioPath, videoID)
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

func (p *Pipeline) fetchTranscripts(ctx context.Context, video *ytlib.Video, videoID string) ([]store.CaptionTrack, string) {
	langs := p.cfg.TranscriptLangs
	if len(langs) == 0 {
		tracks, err := p.client.GetAllTranscripts(ctx, video)
		if err != nil {
			log.Printf("[youtube] transcript fetch failed for %s: %v", videoID, err)
			return nil, ""
		}
		var firstText string
		if len(tracks) > 0 {
			firstText = tracks[0].Text
		}
		return tracks, firstText
	}

	var tracks []store.CaptionTrack
	var firstText string
	for _, lang := range langs {
		track, _, err := p.client.GetTranscript(ctx, video, lang)
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

func (p *Pipeline) exportSubtitles(ctx context.Context, video *ytlib.Video, videoID string) map[string]string {
	subPaths := make(map[string]string)
	langs := p.cfg.TranscriptLangs
	if len(langs) == 0 {
		for _, cap := range video.CaptionTracks {
			langs = append(langs, cap.LanguageCode)
		}
	}
	for _, lang := range langs {
		for _, fmtStr := range p.cfg.SubtitleFormats {
			subFmt, err := ParseSubtitleFormat(fmtStr)
			if err != nil {
				log.Printf("[youtube] unknown subtitle format %s: %v", fmtStr, err)
				continue
			}
			path, err := p.client.ExportSubtitles(ctx, video, lang, subFmt, p.cfg.SubtitleOutputDir)
			if err != nil {
				log.Printf("[youtube] subtitle export %s/%s failed for %s: %v", lang, fmtStr, videoID, err)
				continue
			}
			key := lang + "." + fmtStr
			subPaths[key] = path
		}
	}
	return subPaths
}

func (p *Pipeline) downloadAudio(ctx context.Context, video *ytlib.Video, videoID string) string {
	if err := CheckYTDLP(); err != nil {
		log.Printf("[youtube] %v", err)
		return ""
	}
	audioPath, err := p.client.DownloadAudio(ctx, video, p.cfg.AudioOutputDir)
	if err != nil {
		log.Printf("[youtube] audio download failed for %s: %v", videoID, err)
		return ""
	}
	return audioPath
}

func (p *Pipeline) transcribeAudio(ctx context.Context, audioPath, videoID string) string {
	if err := transcriber.CheckWhisper(); err != nil {
		log.Printf("[youtube] %v", err)
		return ""
	}
	wt := transcriber.NewWhisperTranscriber(p.cfg.TranscriberModelDir)
	opts := transcriber.Options{
		Model:    p.cfg.TranscriberModel,
		ModelDir: p.cfg.TranscriberModelDir,
		Language: p.cfg.TranscriberLanguage,
		Task:     "transcribe",
	}
	tr, err := wt.Transcribe(ctx, audioPath, opts)
	if err != nil {
		log.Printf("[youtube] transcription failed for %s: %v", videoID, err)
		return ""
	}
	return tr.Text
}

func (p *Pipeline) processPlaylist(ctx context.Context, url, playlistID string) (*store.Result, error) {
	items, err := p.client.GetPlaylistItems(ctx, playlistID)
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

func (p *Pipeline) processChannel(ctx context.Context, url, channelID string) (*store.Result, error) {
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
