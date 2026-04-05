package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"

	"greenclaw/pkg/youtube"
)

type ExtractYoutubeReq struct {
	YoutubeURLs []string `json:"youtube_urls"`
}

type ExtractYoutubeResp struct {
	VideoID    string `json:"video_id"`
	Title      string `json:"title"`
	Transcript string `json:"transcript"`
	Language   string `json:"language"`
	Duration   string `json:"duration"`
	Stored     bool   `json:"stored"`
	Error      string `json:"error,omitempty"`
}

func (d *Dependencies) ExtractYoutube(ctx context.Context, req ExtractYoutubeReq) ([]ExtractYoutubeResp, error) {
	results := make([]ExtractYoutubeResp, len(req.YoutubeURLs))
	const maxConcurrent = 1
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	for i, u := range req.YoutubeURLs {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := d.extractOne(ctx, u)
			if err != nil {
				results[i] = ExtractYoutubeResp{Error: fmt.Sprintf("%s: %v", u, err)}
				return
			}
			results[i] = *resp
		}(i, u)
	}
	wg.Wait()
	return results, nil
}

func (d *Dependencies) extractOne(ctx context.Context, youtubeURL string) (*ExtractYoutubeResp, error) {
	videoID, err := extractVideoID(youtubeURL)
	if err != nil {
		return nil, fmt.Errorf("parse video ID: %w", err)
	}

	meta, video, err := d.YtClient.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("get video metadata: %w", err)
	}

	var transcript, language, duration string
	duration = meta.Duration

	// Try captions first.
	langs := d.Cfg.YouTube.TranscriptLangs
	if len(langs) == 0 {
		langs = []string{"en"}
	}

	var captionFound bool
	if d.Cfg.YouTube.ExtractTranscripts && len(meta.Captions) > 0 {
		for _, lang := range langs {
			track, _, err := d.YtClient.GetTranscript(ctx, video, lang)
			if err == nil && track != nil && track.Text != "" {
				transcript = track.Text
				language = track.LanguageCode
				captionFound = true
				log.Printf("[extract] got caption transcript for %s (lang=%s, %d chars)", videoID, language, len(transcript))
				break
			}
		}
	}

	// Fall back to audio download + whisper transcription.
	if !captionFound && d.Cfg.YouTube.DownloadAudio && d.Cfg.YouTube.TranscribeAudio && d.TranscribeClient != nil {
		log.Printf("[extract] no captions for %s — downloading audio", videoID)
		audioPath, err := d.YtClient.DownloadAudio(ctx, videoID, d.Cfg.YouTube.AudioOutputDir, youtube.Options{
			CookiesFromBrowser: d.Cfg.YouTube.CookiesFromBrowser,
			CookiesFile:        d.Cfg.YouTube.CookiesFile,
			SleepInterval:      d.Cfg.YouTube.SleepInterval,
			MaxSleepInterval:   d.Cfg.YouTube.MaxSleepInterval,
			PlayerClients:      d.Cfg.YouTube.PlayerClients,
			POToken:            d.Cfg.YouTube.POToken,
			JSRuntime:          d.Cfg.YouTube.JSRuntime,
			UserAgent:          d.Cfg.YouTube.UserAgent,
		})
		if err != nil {
			return nil, fmt.Errorf("download audio: %w", err)
		}

		result, err := d.TranscribeClient.Transcribe(ctx, audioPath)
		os.Remove(audioPath) // clean up audio file regardless of transcription result
		if err != nil {
			return nil, fmt.Errorf("transcribe audio: %w", err)
		}
		transcript = result.Text
		language = result.Language
		if duration == "" {
			duration = fmt.Sprintf("%.0fs", result.Duration)
		}
		log.Printf("[extract] whisper transcript for %s (lang=%s, %d chars)", videoID, language, len(transcript))
	}

	if transcript == "" {
		return nil, fmt.Errorf("could not obtain transcript for video %s", videoID)
	}

	resp := &ExtractYoutubeResp{
		VideoID:    videoID,
		Title:      meta.Title,
		Transcript: transcript,
		Language:   language,
		Duration:   duration,
	}

	// Persist to graph DB.
	if d.GraphDB != nil {
		doc := map[string]interface{}{
			"title":      meta.Title,
			"url":        youtubeURL,
			"transcript": transcript,
			"language":   language,
			"duration":   duration,
			"processed":  false,
			"category":   "",
		}
		if err := d.GraphDB.UpsertNode(ctx, "Video", videoID, doc); err != nil {
			log.Printf("[extract] warn: failed to store video %s in graph DB: %v", videoID, err)
		} else {
			resp.Stored = true
			log.Printf("[extract] stored video %s in graph DB", videoID)
		}
	}

	return resp, nil
}

// extractVideoID parses a YouTube video ID from a URL or bare ID.
func extractVideoID(raw string) (string, error) {
	if !strings.Contains(raw, "/") && !strings.Contains(raw, ".") {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	// youtu.be/<id>
	if strings.Contains(u.Host, "youtu.be") {
		id := strings.TrimPrefix(u.Path, "/")
		if id == "" {
			return "", fmt.Errorf("no video ID in URL")
		}
		return id, nil
	}
	// youtube.com/watch?v=<id>  or  youtube.com/shorts/<id>
	if v := u.Query().Get("v"); v != "" {
		return v, nil
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	for i, p := range parts {
		if (p == "shorts" || p == "embed" || p == "v") && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("could not extract video ID from URL: %s", raw)
}
