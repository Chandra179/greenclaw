package youtube

import (
	"context"
	"log"
	"time"

	"golang.org/x/sync/errgroup"

	"greenclaw/internal/store"
	"greenclaw/pkg/transcribe"
	"greenclaw/pkg/ytdl"

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
	YTDLOptions        ytdl.Options
	NumCtx             int // optional; overrides LLM context window size for this request
}

func ProcessVideo(ctx context.Context, client *Client, cfg PipelineConfig, t transcribe.Client, url, videoID string) (*store.Result, error) {
	ytData, video, err := client.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return nil, err
	}

	r := &store.Result{
		URL:         url,
		Title:       video.Title,
		Description: video.Description,
		FetchedAt:   time.Now(),
	}

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
		r.FilePath = audioPath
	}

	// Stage 4: transcribe audio (sequential, requires audio)
	var whisperText string
	if audioPath != "" && cfg.TranscribeAudio {
		whisperText = transcribeAudio(ctx, t, audioPath, videoID)
	}

	// Choose text priority: captions > whisper
	if captionText != "" {
		r.Text = captionText
	} else if whisperText != "" {
		r.Text = whisperText
	}

	// Stage 5: LLM processing (sequential, requires transcript text)
	if r.Text != "" && len(cfg.ProcessingStyles) > 0 && llm != nil {
		for _, style := range cfg.ProcessingStyles {
			pr, err := llm.Process(ctx, llms.Request{
				Style:      style,
				Title:      video.Title,
				Text:       r.Text,
				CacheKey:   videoID,
				ProgressCh: cfg.ProgressCh,
				NumCtx:     cfg.NumCtx,
			})
			if err != nil {
				log.Printf("[youtube] processing style %s failed for %s: %v", style, videoID, err)
				continue
			}
			ytData.Processing = append(ytData.Processing, store.ProcessingResult{
				Style:   string(pr.Style),
				Content: pr.Content,
			})
		}
		if len(ytData.Processing) < len(cfg.ProcessingStyles) {
			log.Printf("[youtube] %d/%d processing styles succeeded for %s",
				len(ytData.Processing), len(cfg.ProcessingStyles), videoID)
		}
	}

	r.YouTube = ytData

	return r, nil
}

func fetchTranscripts(ctx context.Context, client *Client, cfg PipelineConfig,
	video *ytlib.Video, videoID string) ([]store.CaptionTrack, string) {
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
	if err := ytdl.CheckYTDLP(); err != nil {
		log.Printf("[youtube] %v", err)
		return ""
	}
	audioPath, err := ytdl.DownloadAudio(ctx, video.ID, cfg.AudioOutputDir, cfg.YTDLOptions)
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
