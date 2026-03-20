package scraper

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"greenclaw/internal/router"
	"greenclaw/internal/store"
	"greenclaw/internal/transcriber"
	"greenclaw/internal/youtube"
)

// fetchYouTube handles YouTube URL extraction, orchestrating metadata,
// transcript, audio, and subtitle extraction.
func (s *Scraper) fetchYouTube(ctx context.Context, url string, ytType router.YouTubeURLType, id string) (*store.Result, error) {
	ytClient := youtube.NewClient(s.client)

	switch ytType {
	case router.YouTubePlaylist:
		return s.fetchYouTubePlaylist(ctx, ytClient, url, id)
	case router.YouTubeChannel:
		return s.fetchYouTubeChannel(ctx, url, id)
	default:
		return s.fetchYouTubeVideo(ctx, ytClient, url, id)
	}
}

func (s *Scraper) fetchYouTubeVideo(ctx context.Context, ytClient *youtube.Client, url, videoID string) (*store.Result, error) {
	// Get metadata (must run first — everything else depends on it)
	ytData, video, err := ytClient.GetVideoMetadata(ctx, videoID)
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

	var mu sync.Mutex // guards concurrent writes to ytData fields
	var captionText string
	var whisperText string

	g, gctx := errgroup.WithContext(ctx)

	// Group A: Extract transcripts (independent)
	if s.cfg.YouTube.ExtractTranscripts {
		g.Go(func() error {
			langs := s.cfg.YouTube.TranscriptLangs
			if len(langs) == 0 {
				tracks, err := ytClient.GetAllTranscripts(gctx, video)
				if err != nil {
					log.Printf("[youtube] transcript fetch failed for %s: %v", videoID, err)
					return nil
				}
				mu.Lock()
				ytData.Captions = tracks
				mu.Unlock()
				if len(tracks) > 0 {
					captionText = tracks[0].Text
				}
			} else {
				var tracks []store.CaptionTrack
				var firstText string
				for _, lang := range langs {
					track, _, err := ytClient.GetTranscript(gctx, video, lang)
					if err != nil {
						log.Printf("[youtube] transcript %s failed for %s: %v", lang, videoID, err)
						continue
					}
					tracks = append(tracks, *track)
					if firstText == "" {
						firstText = track.Text
					}
				}
				mu.Lock()
				// Update existing captions or append new ones
				for _, track := range tracks {
					updated := false
					for i, cap := range ytData.Captions {
						if cap.LanguageCode == track.LanguageCode {
							ytData.Captions[i] = track
							updated = true
							break
						}
					}
					if !updated {
						ytData.Captions = append(ytData.Captions, track)
					}
				}
				mu.Unlock()
				captionText = firstText
			}
			return nil
		})
	}

	// Group B: Export subtitles (independent)
	if s.cfg.YouTube.ExportSubtitles {
		g.Go(func() error {
			subPaths := make(map[string]string)
			langs := s.cfg.YouTube.TranscriptLangs
			if len(langs) == 0 {
				for _, cap := range video.CaptionTracks {
					langs = append(langs, cap.LanguageCode)
				}
			}
			for _, lang := range langs {
				for _, fmtStr := range s.cfg.YouTube.SubtitleFormats {
					subFmt, err := youtube.ParseSubtitleFormat(fmtStr)
					if err != nil {
						log.Printf("[youtube] unknown subtitle format %s: %v", fmtStr, err)
						continue
					}
					path, err := ytClient.ExportSubtitles(gctx, video, lang, subFmt, s.cfg.YouTube.SubtitleOutputDir)
					if err != nil {
						log.Printf("[youtube] subtitle export %s/%s failed for %s: %v", lang, fmtStr, videoID, err)
						continue
					}
					key := lang + "." + fmtStr
					subPaths[key] = path
				}
			}
			mu.Lock()
			ytData.SubtitlePaths = subPaths
			mu.Unlock()
			return nil
		})
	}

	// Group C: Download audio → transcribe (sequential within group)
	if s.cfg.YouTube.DownloadAudio {
		g.Go(func() error {
			if err := youtube.CheckYTDLP(); err != nil {
				log.Printf("[youtube] %v", err)
				return nil
			}
			audioPath, err := ytClient.DownloadAudio(gctx, video, s.cfg.YouTube.AudioOutputDir)
			if err != nil {
				log.Printf("[youtube] audio download failed for %s: %v", videoID, err)
				return nil
			}
			mu.Lock()
			ytData.AudioPath = audioPath
			mu.Unlock()
			result.FilePath = audioPath

			// Transcribe if configured
			if s.cfg.YouTube.TranscribeAudio {
				if err := transcriber.CheckWhisper(); err != nil {
					log.Printf("[youtube] %v", err)
					return nil
				}
				wt := transcriber.NewWhisperTranscriber(s.cfg.Transcriber.ModelDir)
				opts := transcriber.Options{
					Model:    s.cfg.Transcriber.Model,
					ModelDir: s.cfg.Transcriber.ModelDir,
					Language: s.cfg.Transcriber.Language,
					Task:     "transcribe",
				}
				tr, err := wt.Transcribe(gctx, audioPath, opts)
				if err != nil {
					log.Printf("[youtube] transcription failed for %s: %v", videoID, err)
					return nil
				}
				whisperText = tr.Text
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
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

func (s *Scraper) fetchYouTubePlaylist(ctx context.Context, ytClient *youtube.Client, url, playlistID string) (*store.Result, error) {
	items, err := ytClient.GetPlaylistItems(ctx, playlistID)
	if err != nil {
		return nil, err
	}

	// Build a title/description summary
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

func (s *Scraper) fetchYouTubeChannel(ctx context.Context, url, channelID string) (*store.Result, error) {
	// Channel URLs: we return basic info since resolving uploads playlist
	// requires additional API calls. The channel ID/handle is captured.
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
