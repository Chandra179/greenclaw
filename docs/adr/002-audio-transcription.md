# ADR 002: Audio Transcription via faster-whisper

## Status

Accepted

## Date

2026-03-20

## Context

greenclaw can download YouTube audio and extract captions from YouTube's API. However, many videos lack captions entirely — auto-generated captions are unavailable for some languages, live streams, and older content. When captions are missing, the spoken content of downloaded audio files cannot be extracted.

Adding speech-to-text capability would allow greenclaw to transcribe any downloaded audio, filling the gap when YouTube captions are unavailable.

## Decision

Add a new `internal/transcriber` package that wraps **faster-whisper** as a subprocess, following the same pattern used for yt-dlp in `internal/youtube/audio.go`.

### Why a separate package?

Audio transcription is **audio-agnostic** — it operates on any audio file, not just YouTube downloads. Placing it in `internal/youtube` would couple transcription to YouTube. The codebase already follows domain-based package separation:

| Package | Domain |
|---------|--------|
| `internal/youtube` | YouTube API interactions |
| `internal/fetcher` | HTTP content fetching |
| `internal/browser` | Headless browser automation |
| **`internal/transcriber`** | **Speech-to-text (new)** |

### Engine: faster-whisper (CTranslate2)

faster-whisper is a reimplementation of OpenAI's Whisper using CTranslate2 for inference. It is invoked as a subprocess via its CLI, matching the existing yt-dlp pattern.

### Package structure

```
internal/transcriber/
  transcriber.go      # Interface, Options, Result types
  whisper.go          # faster-whisper subprocess implementation
  whisper_test.go     # Tests
```

**Key types:**

```go
type Options struct {
    Model    string // Model size: tiny, base, small, medium, large-v3
    ModelDir string // Directory containing model files
    Language string // ISO 639-1 code, empty for auto-detect
    Task     string // "transcribe" or "translate" (to English)
}

type Result struct {
    Text     string  // Transcribed text
    Language string  // Detected or specified language
    Duration float64 // Audio duration in seconds
}

type Transcriber interface {
    Transcribe(ctx context.Context, audioPath string, opts Options) (*Result, error)
}
```

`CheckWhisper()` validates the faster-whisper CLI is in PATH, following the `CheckYTDLP()` pattern from `internal/youtube/audio.go`.

### Integration point

In `internal/scraper/youtube.go`, after the audio download block (line ~100):

```
if cfg.YouTube.TranscribeAudio && ytData.AudioPath != "" {
    → CheckWhisper()
    → NewWhisperTranscriber(cfg.Transcriber.ModelDir)
    → Transcribe(ctx, audioPath, opts)
    → Store result in ytData.TranscriptFromAudio
    → Use as Result.Text fallback when no captions exist
}
```

Transcription runs **after** audio download and **only when** `TranscribeAudio` is enabled and an audio file exists. It acts as a fallback — if captions were already extracted, they take priority for `Result.Text`.

### Configuration

New top-level `TranscriberConfig` in `internal/config/config.go`:

```go
type TranscriberConfig struct {
    Model    string `yaml:"model"`     // default: "base"
    ModelDir string `yaml:"model_dir"` // default: "/models/whisper"
    Language string `yaml:"language"`  // default: "" (auto-detect)
}
```

New field in `YouTubeConfig`: `TranscribeAudio bool`

New CLI flag: `--youtube-transcribe`

### Store addition

New field in `store.YouTubeData`: `TranscriptFromAudio string`

This is kept separate from `Captions` to distinguish YouTube-provided captions from locally-generated transcriptions.

## Alternatives Considered

### OpenAI Whisper (original Python implementation)

- **Pros:** Reference implementation, well-documented, widely used
- **Cons:** Slow on CPU — 10-50x realtime for base model. Uses PyTorch which adds ~2 GB to the Docker image.
- **Verdict:** Rejected. faster-whisper achieves the same accuracy at 4-8x the speed on CPU with a smaller footprint.

### whisper.cpp with Go bindings

- **Pros:** No Python dependency for transcription, very fast, pure C++
- **Cons:** Requires CGO, which complicates cross-compilation and the Docker build. Go bindings (go-whisper) are less mature and have breaking API changes between versions.
- **Verdict:** Rejected. CGO adds build complexity. The subprocess pattern is proven (yt-dlp) and keeps the Go binary static.

### External API (OpenAI Whisper API, Google Speech-to-Text)

- **Pros:** No local compute needed, high accuracy
- **Cons:** Per-minute cost ($0.006/min for OpenAI), requires network access, sends audio to third parties (privacy concern), rate limits.
- **Verdict:** Rejected. greenclaw is designed for local operation. Adding API dependencies contradicts the "no external services" philosophy.

### Embedding transcription in `internal/youtube`

- **Pros:** Fewer packages, simpler import graph
- **Cons:** Couples transcription to YouTube. If greenclaw later transcribes audio from other sources (podcasts, uploaded files), the code would be in the wrong package.
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

- **Positive:** Videos without captions become searchable/extractable. The transcriber module is reusable for non-YouTube audio. Subprocess pattern keeps the Go binary simple and static.
- **Negative:** Adds ~200 MB to Docker image (CTranslate2 + dependencies). CPU transcription is slow for long audio. Model files must be downloaded on first use or pre-populated in the volume.
- **Risks:** faster-whisper CLI interface could change between versions. Mitigation: pin the version in requirements/Dockerfile and test against it.

## Implementation

Implemented. All files exist as designed:

- `internal/transcriber/transcriber.go` — `Transcriber` interface, `Options`, `Result` types
- `internal/transcriber/whisper.go` — `WhisperTranscriber`, `CheckWhisper()`
- `internal/transcriber/whisper_test.go`
- `internal/config/config.go` — `TranscriberConfig` and `TranscribeAudio` in `YouTubeConfig`
- `internal/store/youtube.go` — `TranscriptFromAudio string` field
- `internal/scraper/youtube.go` — transcription wired after audio download, used as `Result.Text` fallback
- `main.go` — `--youtube-transcribe` flag
- `config.yaml` — `transcriber` section
