package scraper

import (
	"context"
	"log"
	"strings"
	"time"

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
	// Get metadata
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

	// Extract transcripts
	if s.cfg.YouTube.ExtractTranscripts {
		langs := s.cfg.YouTube.TranscriptLangs
		if len(langs) == 0 {
			// Fetch all available transcripts
			tracks, err := ytClient.GetAllTranscripts(ctx, video)
			if err != nil {
				log.Printf("[youtube] transcript fetch failed for %s: %v", videoID, err)
			} else {
				ytData.Captions = tracks
				// Use first track's text as Result.Text
				if len(tracks) > 0 {
					result.Text = tracks[0].Text
				}
			}
		} else {
			for _, lang := range langs {
				track, _, err := ytClient.GetTranscript(ctx, video, lang)
				if err != nil {
					log.Printf("[youtube] transcript %s failed for %s: %v", lang, videoID, err)
					continue
				}
				// Update caption in ytData
				updated := false
				for i, cap := range ytData.Captions {
					if cap.LanguageCode == lang {
						ytData.Captions[i] = *track
						updated = true
						break
					}
				}
				if !updated {
					ytData.Captions = append(ytData.Captions, *track)
				}
				// First requested language becomes Result.Text
				if result.Text == "" {
					result.Text = track.Text
				}
			}
		}
	}

	// Download audio if configured (requires yt-dlp)
	if s.cfg.YouTube.DownloadAudio {
		if err := youtube.CheckYTDLP(); err != nil {
			log.Printf("[youtube] %v", err)
			return result, nil
		}
		audioPath, err := ytClient.DownloadAudio(ctx, video, s.cfg.YouTube.AudioOutputDir)
		if err != nil {
			log.Printf("[youtube] audio download failed for %s: %v", videoID, err)
		} else {
			ytData.AudioPath = audioPath
			result.FilePath = audioPath
		}
	}

	// Transcribe audio if configured and audio was downloaded
	if s.cfg.YouTube.TranscribeAudio && ytData.AudioPath != "" {
		if err := transcriber.CheckWhisper(); err != nil {
			log.Printf("[youtube] %v", err)
		} else {
			wt := transcriber.NewWhisperTranscriber(s.cfg.Transcriber.ModelDir)
			opts := transcriber.Options{
				Model:    s.cfg.Transcriber.Model,
				ModelDir: s.cfg.Transcriber.ModelDir,
				Language: s.cfg.Transcriber.Language,
				Task:     "transcribe",
			}
			tr, err := wt.Transcribe(ctx, ytData.AudioPath, opts)
			if err != nil {
				log.Printf("[youtube] transcription failed for %s: %v", videoID, err)
			} else {
				ytData.TranscriptFromAudio = tr.Text
				// Use as fallback when no captions exist
				if result.Text == "" {
					result.Text = tr.Text
				}
			}
		}
	}

	// Export subtitles if configured
	if s.cfg.YouTube.ExportSubtitles {
		ytData.SubtitlePaths = make(map[string]string)
		langs := s.cfg.YouTube.TranscriptLangs
		if len(langs) == 0 {
			// Export all available
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
				path, err := ytClient.ExportSubtitles(ctx, video, lang, subFmt, s.cfg.YouTube.SubtitleOutputDir)
				if err != nil {
					log.Printf("[youtube] subtitle export %s/%s failed for %s: %v", lang, fmtStr, videoID, err)
					continue
				}
				key := lang + "." + fmtStr
				ytData.SubtitlePaths[key] = path
			}
		}
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
