# greenclaw

YouTube transcript extraction pipeline. Extracts transcripts from YouTube videos (via captions or audio transcription) and stores results in SQLite.

## Data flow

```
POST /extract/youtube
  → youtube.Client.GetVideoMetadata(videoID)
  → if captions: GetTranscript(video, langCode)
    else:        DownloadAudio(videoID, dir) → transcribe.Client.Transcribe(audioPath)
  → storage.Client.StoreVideo(record)          [SQLite]
```

## Key packages

- **`pkg/youtube`** — wraps `kkdai/youtube/v2`; caption fetching + yt-dlp audio download.
- **`pkg/transcribe`** — HTTP client for whisper-service FastAPI endpoint.
- **`pkg/storage`** — SQLite document store for transcript results.
- **`internal/config`** — YAML config with defaults.
- **`internal/router`** — Gin routes: `POST /extract/youtube`, `GET /swagger/*`.
- **`internal/service`** — `ExtractYoutube` orchestrator.
- **`cmd/app`** — HTTP server entrypoint.

## Infrastructure

- **whisper-service/** — Python FastAPI + faster-whisper (GPU).
- **docker-compose.yaml** — runs `greenclaw` (port 8080). Uses `host.docker.internal` to reach whisper-service on the host.
