# ADR 002: Audio Transcription via faster-whisper

## Status

Accepted

## Date

2026-03-20

## Context

greenclaw can download YouTube audio and extract captions from YouTube's API. However, many videos lack captions entirely — auto-generated captions are unavailable for some languages, live streams, and older content. When captions are missing, the spoken content of downloaded audio files cannot be extracted.

Adding speech-to-text capability would allow greenclaw to transcribe any downloaded audio, filling the gap when YouTube captions are unavailable.

## Decision

Add a new `internal/transcriber` package that calls a **dedicated Python HTTP service** (`whisper-service/`) running faster-whisper. The Go binary POSTs audio files to the service over HTTP rather than invoking a local subprocess.

### Why a separate service instead of a subprocess?

The original design wrapped the faster-whisper CLI as a subprocess inside the Go container. This was replaced with an HTTP service to enable **GPU access** — the Go container runs in Docker without GPU passthrough, while the whisper service runs on the host machine (or a GPU-capable container) with CUDA access. A subprocess confined to the Go container would always be CPU-only.

### Why a separate package?

Audio transcription is **audio-agnostic** — it operates on any audio file, not just YouTube downloads. Placing it in `internal/youtube` would couple transcription to YouTube. The codebase already follows domain-based package separation:

| Package | Domain |
|---------|--------|
| `internal/youtube` | YouTube API interactions |
| `internal/fetcher` | HTTP content fetching |
| `internal/browser` | Headless browser automation |
| **`internal/transcriber`** | **Speech-to-text** |

### Engine: faster-whisper (CTranslate2) via FastAPI

`whisper-service/` is a Python FastAPI application wrapping faster-whisper. It exposes two endpoints:

- `GET /health` — returns model/device info
- `POST /transcribe` — accepts a multipart audio file upload, returns `{ text, language, duration }`

The service defaults to `medium` model, `cuda` device, and `int8_float16` compute type (safe for 6 GB VRAM). All are configurable via environment variables.

### Package structure

```
internal/transcriber/
  client.go           # Client interface, Result type, New() factory
  http.go             # HTTPTranscriber — streams audio to remote service
  whisper.go          # WhisperTranscriber — legacy subprocess fallback
  whisper_test.go     # Tests

whisper-service/
  main.py             # FastAPI app wrapping faster-whisper
  requirements.txt    # fastapi, uvicorn, faster-whisper, python-multipart
  Makefile            # `make py` → uvicorn on port 9000
```

**Key types:**

```go
type Result struct {
    Text     string  // Transcribed text
    Language string  // Detected or specified language
    Duration float64 // Audio duration in seconds
}

type Client interface {
    Transcribe(ctx context.Context, audioPath string) (*Result, error)
}
```

`New(cfg)` returns an `HTTPTranscriber` pointed at `cfg.Endpoint`. Language is baked in at construction time (no per-call options).

### Integration point

In `internal/scraper/youtube.go`, after the audio download block:

```
if cfg.YouTube.TranscribeAudio && ytData.AudioPath != "" {
    → transcriber.New(cfg.Transcriber)
    → Transcribe(ctx, audioPath)
    → Use as Result.Text fallback when no captions exist
}
```

Transcription runs **after** audio download and **only when** `TranscribeAudio` is enabled and an audio file exists. If captions were already extracted, they take priority for `Result.Text`.

### Deployment

The whisper service runs outside the Go container. In a typical local setup it runs on the host:

```bash
cd whisper-service && make py   # starts on port 9000
```

The Go container reaches it via `host.docker.internal:9000` (configured via `extra_hosts` in `docker-compose.yaml`).

### Configuration

`TranscriberConfig` in `internal/config/config.go`:

```go
type TranscriberConfig struct {
    Endpoint string `yaml:"endpoint"` // URL of remote whisper HTTP service
    Timeout  string `yaml:"timeout"`  // Request timeout, e.g. "5m"
    Language string `yaml:"language"` // ISO 639-1 code, empty for auto-detect
}
```

New field in `YouTubeConfig`: `TranscribeAudio bool`

### Store addition

Whisper transcription text is used as a fallback for `Result.Text` when no YouTube captions are available. It is not stored separately in `YouTubeData`.

## Alternatives Considered

### faster-whisper as in-container subprocess (original design)

- **Pros:** No network hop, simpler configuration, no separate process to manage
- **Cons:** The Go container runs without GPU passthrough. CPU-only whisper is 4-8x slower than GPU. Model files would need to be in the container or a shared volume.
- **Verdict:** Superseded. GPU access requires running outside the Go container; HTTP service is the natural boundary.

### OpenAI Whisper (original Python implementation)

- **Pros:** Reference implementation, well-documented, widely used
- **Cons:** Slow on CPU — 10-50x realtime for base model. Uses PyTorch which adds ~2 GB to the image.
- **Verdict:** Rejected. faster-whisper achieves the same accuracy at 4-8x the speed on CPU, and better GPU performance with CTranslate2.

### whisper.cpp with Go bindings

- **Pros:** No Python dependency, very fast, pure C++
- **Cons:** Requires CGO, complicating cross-compilation and Docker builds. Go bindings (go-whisper) are less mature with breaking API changes.
- **Verdict:** Rejected. CGO adds build complexity and doesn't solve the GPU access problem.

### External API (OpenAI Whisper API, Google Speech-to-Text)

- **Pros:** No local compute needed, high accuracy
- **Cons:** Per-minute cost, requires network access, sends audio to third parties (privacy concern), rate limits.
- **Verdict:** Rejected. greenclaw is designed for local operation.

### Embedding transcription in `internal/youtube`

- **Pros:** Fewer packages, simpler import graph
- **Cons:** Couples transcription to YouTube.
- **Verdict:** Rejected. Domain separation is a core architectural pattern in this codebase.

## Model Size Tradeoffs

| Model | Download | CPU Speed* | Accuracy | Use Case |
|-------|----------|-----------|----------|----------|
| tiny | 39 MB | ~32x realtime | Lowest | Quick previews, keyword spotting |
| **base** | **74 MB** | **~16x realtime** | **Good** | **Default — balanced speed/quality** |
| small | 244 MB | ~6x realtime | Better | When accuracy matters more than speed |
| medium | 769 MB | ~2x realtime | High | Long-form content, multiple languages |
| large-v3 | 1.5 GB | ~1x realtime | Highest | Maximum accuracy, production use |

*Approximate speeds with faster-whisper on a 4-core CPU.

The `base` model is the default: it downloads in seconds, transcribes at 16x realtime (a 10-minute video takes ~37 seconds), and provides good accuracy for most English content. Users can override via config or CLI flag.

## Consequences

- **Positive:** GPU transcription is available without rebuilding the Go container. The transcriber module is reusable for non-YouTube audio. Model management is decoupled from the Go binary.
- **Negative:** Requires running a second process (`whisper-service`). Adds a network dependency between the Go container and the service. If the service is not running, `transcribe_audio` silently fails.
- **Risks:** Audio files must be accessible to the Go container for upload (already true via `downloads/` volume). faster-whisper API changes between versions — pin in `requirements.txt`.

## Implementation

Implemented:

- `whisper-service/main.py` — FastAPI app; `POST /transcribe`, `GET /health`
- `whisper-service/requirements.txt` — `fastapi`, `uvicorn`, `faster-whisper`, `python-multipart`
- `whisper-service/Makefile` — `make py` starts the service on port 9000
- `internal/transcriber/client.go` — `Client` interface, `Result` type, `New()` factory
- `internal/transcriber/http.go` — `HTTPTranscriber` (streams audio via multipart POST)
- `internal/transcriber/whisper.go` — `WhisperTranscriber` (legacy subprocess, kept for reference)
- `internal/transcriber/whisper_test.go`
- `internal/config/config.go` — `TranscriberConfig` (`Endpoint`, `Timeout`, `Language`) and `TranscribeAudio` in `YouTubeConfig`
- `internal/scraper/youtube.go` — transcription wired after audio download, used as `Result.Text` fallback
- `docker-compose.yaml` — `extra_hosts: host.docker.internal:host-gateway` so the container can reach the host-side service
