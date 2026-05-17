# CLAUDE.md

## Commands

```bash
go build ./...                     # build
go test ./...                      # run all tests
go run ./cmd/app                   # run HTTP server
make build                         # build binary
make br                            # full rebuild: docker build + docker compose up
make r                             # docker compose up -d
```

## Architecture

**greenclaw** is a YouTube transcript extraction pipeline. It extracts transcripts (or downloads audio for transcription) and stores structured results in SQLite.

### Data flow

```
POST /extract/youtube
  → youtube.Client.GetVideoMetadata(videoID)
  → if captions: GetTranscript(video, langCode)
    else:        DownloadAudio(videoID, dir) → transcribe.Client.Transcribe(audioPath)
  → storage.Client.StoreVideo(record)
```

### Key packages

- **`pkg/youtube`** — wraps `kkdai/youtube/v2`; caption fetching + yt-dlp audio download with 5-strategy fallback (web → mweb → android_vr → default → no-webpage) for bot detection resilience.
- **`pkg/transcribe`** — HTTP client for the whisper-service FastAPI endpoint; returns `{text, language, duration}`.
- **`pkg/storage`** — SQLite document store; `videos` table with upsert semantics.
- **`pkg/httpclient`** — `http.Client` factory that injects a browser-like User-Agent.
- **`internal/config`** — YAML config with defaults; auto-creates `config.yaml` on first run.
- **`internal/router`** — Gin routes: `POST /extract/youtube`, `GET /swagger/*`.
- **`internal/service`** — Orchestrator (`ExtractYoutube`) that wires the pkg clients together.

### Concurrency model

Two independent semaphore channels from config:
- `httpSem` (default 20) — held during HTTP fetches
- `browserSem` (default 5) — acquired after `httpSem` is released, to avoid deadlock during escalation

Retry with exponential backoff; graceful context cancellation.

### Infrastructure

- **whisper-service/** — Python FastAPI + faster-whisper (GPU). Set `WHISPER_MODEL=small` by default to conserve VRAM. Single worker (`num_workers=1`) to avoid multi-process VRAM pressure.
- **docker-compose.yaml** — runs `greenclaw` (port 8080). Uses `host.docker.internal` to reach whisper-service on the host.
