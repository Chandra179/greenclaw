# greenclaw

A detect-first, escalate-later web scraper written in Go. It attempts the lightest possible extraction method for each URL and only launches a headless browser when simpler approaches fail.

---

## Quick Start

**Run with Docker:**

```bash
curl -X POST http://localhost:8080/extract \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://www.youtube.com/watch?v=bqXvzJclaAg"}'
```

**Run locally:**

```bash
go run main.go <url>
go run main.go --urls-file urls.txt
go run main.go --output json <url>
```

**Prerequisites for full functionality:**

| Tool | Required for |
| ---- | ------------ |
| `yt-dlp` | YouTube audio download (`download_audio: true`) |
| `ffmpeg` | Opus audio conversion (optional) |
| `faster-whisper` | Audio transcription (`transcribe_audio: true`) |
| Chromium | Stage 2 browser escalation (managed by go-rod) |

---

## How It Works

Every URL is classified by content type before any parsing begins:

- **YouTube URLs** — extracted via `kkdai/youtube` + `yt-dlp`, no browser launched
- **PDF / images** — streamed directly to disk
- **JSON / XML** — HTTP GET and unmarshal
- **HTML** — plain HTTP + goquery first; headless Chrome only if blocked or JS-required

See [docs/architecture.md](docs/architecture.md) for the full data flow and concurrency model.

---

## Configuration

Copy `config.yaml` and adjust as needed:

```yaml
port: 8080
http_concurrency: 20
browser_concurrency: 5
timeout: 30s
retry_attempts: 3
recycle_after: 100

youtube:
  extract_transcripts: true
  transcript_langs: []          # empty = all languages
  download_audio: false
  audio_output_dir: downloads/audio
  transcribe_audio: false
  export_subtitles: false
  subtitle_formats: [srt]
  subtitle_output_dir: downloads/subtitles

transcriber:
  model: base                   # tiny, base, small, medium, large-v3
  model_dir: /models/whisper
  language: ""                  # empty = auto-detect
```

---

## Documentation

| Doc | Description |
| --- | ----------- |
| [docs/architecture.md](docs/architecture.md) | System architecture, data flow, concurrency model |
| [docs/youtube.md](docs/youtube.md) | YouTube feature reference: metadata, transcripts, audio, subtitles |
| [docs/dependencies.md](docs/dependencies.md) | All dependencies and why each was chosen |
| [docs/adr/index.md](docs/adr/index.md) | Architecture Decision Records index |

---

## Development

```bash
go build ./...          # build
go test ./...           # run all tests
go test ./internal/...  # run tests for a specific package
```
