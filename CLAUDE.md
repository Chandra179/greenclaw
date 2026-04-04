# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build ./...                     # build
go test ./...                      # run all tests
go test ./pkg/youtube/...          # run tests in a specific package
go run ./cmd/app                   # run HTTP server
make build                         # build binary (runs swag init first)
make br                            # full rebuild: swag + docker build + docker compose up
make r                             # docker compose up -d
```

## Architecture

**greenclaw** is a YouTube content extraction and knowledge graph pipeline. It extracts transcripts (or downloads audio for transcription), processes text through an LLM, and stores structured results in ArangoDB.

### Data flow

```
POST /extract/youtube
  → youtube.Client.GetVideoMetadata(videoID)
  → if captions: GetTranscript(video, langCode)
    else:        DownloadAudio(videoID, dir) → transcribe.Client.Transcribe(audioPath)
  → llm.Client.Chat(transcript, schema)   [optional, per processing_styles]
  → graphdb.Client.UpsertVertex/Edge(...)  [optional, if graph.enabled]
```

### Key packages

- **`pkg/youtube`** — wraps `kkdai/youtube/v2`; caption fetching + yt-dlp audio download with 5-strategy fallback (web → mweb → android_vr → default → no-webpage) for bot detection resilience.
- **`pkg/transcribe`** — HTTP client for the whisper-service FastAPI endpoint; returns `{text, language, duration}`.
- **`pkg/llm`** — HTTP client for Ollama (or compatible); chunks text via `pkg/chunker` before sending.
- **`pkg/chunker`** — `RecursiveChunker` splits at semantic boundaries (`\n\n → \n → . → word`); configurable ChunkSize + Overlap.
- **`pkg/graphdb`** — ArangoDB driver v2 wrapper; collections: `videos`, `entities`, `mentions` (edge), `related_to` (edge), `results`. Supports 768-dim vector embeddings (requires ArangoDB 3.12+).
- **`pkg/httpclient`** — `http.Client` factory that injects a browser-like User-Agent.
- **`internal/config`** — YAML config with defaults; auto-creates `config.yaml` on first run.
- **`internal/router`** — Gin routes: `POST /extract/youtube`, `POST /extract/graph`, `GET /swagger/*`.
- **`internal/service`** — Orchestrator stubs (`ExtractYoutube`, `BuildGraph`) that wire the pkg clients together. **Currently unimplemented.**
- **`cmd/app`** — HTTP server entrypoint; reads config, calls `internal/server.NewServer`.

### Concurrency model

Two independent semaphore channels from config:
- `httpSem` (default 20) — held during HTTP fetches
- `browserSem` (default 5) — acquired after `httpSem` is released, to avoid deadlock during escalation

Retry with exponential backoff; graceful context cancellation.

### Infrastructure

- **whisper-service/** — Python FastAPI + faster-whisper (GPU). Set `WHISPER_MODEL=small` by default to conserve VRAM. Single worker (`num_workers=1`) to avoid multi-process VRAM pressure.
- **docker-compose.yaml** — runs `greenclaw` (port 8080) + `arangodb:3.12` (port 8529, no-auth). Uses `host.docker.internal` to reach Ollama and whisper-service on the host.

### Current state

`internal/service/extract_youtube.go` and `build_graph.go` are stubs. No `*_test.go` files exist yet.
