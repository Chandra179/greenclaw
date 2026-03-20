# Dependencies

---

## Go Modules (Direct)

### `github.com/PuerkitoBio/goquery`

CSS-selector-based HTML parsing, modelled after jQuery. Used in `internal/fetcher` for Stage 1 HTML extraction.

**Why this over alternatives:**
- Pure Go, no CGO — keeps the binary static and the Docker build simple
- CSS selector API is more readable than walking the `golang.org/x/net/html` AST directly
- Battle-tested: one of the most widely used HTML scraping libraries in the Go ecosystem

### `github.com/go-rod/rod`

Pure-Go Chrome DevTools Protocol (CDP) client for headless browser automation. Used in `internal/browser` for Stage 2 escalation.

**Why this over alternatives:**
- Pure Go (no Node.js, no Playwright, no Puppeteer) — matches the codebase's static-binary preference
- Manages Chrome download and lifecycle internally via `launcher`
- Built-in browser pool, tab recycling, and request interception APIs
- `go-rod/stealth` companion plugin patches fingerprint vectors at the CDP level

### `github.com/go-rod/stealth`

Stealth plugin for go-rod that patches common browser fingerprint vectors before any page interaction: `navigator.webdriver`, canvas noise, WebRTC behaviour, screen resolution spoofing, and others.

**Why a separate package:** The stealth patches are JavaScript injected at the CDP level before page load. Bundling them into `go-rod/rod` itself would make stealth opt-out by default; keeping it separate makes it opt-in and auditable.

### `github.com/kkdai/youtube/v2`

YouTube API client that resolves video metadata, stream manifests, playlist items, and caption track URLs without requiring an API key. Used in `internal/youtube`.

**Why this over alternatives:**
- No API key required — YouTube's internal client API is used directly
- Actively maintained and tracks YouTube's anti-bot changes
- Handles caption track URL resolution, which is non-trivial to implement correctly
- Does **not** handle audio stream downloads (requires yt-dlp for that — see External Tools below)

### `gopkg.in/yaml.v3`

YAML decoder used to load `config.yaml` into `internal/config.Config`.

**Why yaml.v3 over json or toml:** YAML is the most readable format for the multi-section config this project has (scraper, YouTube, transcriber). v3 over v2 because it correctly handles multi-document YAML and has better type error messages.

---

## External Tools

### `yt-dlp`

CLI tool for downloading YouTube audio streams. Used in `internal/youtube/audio.go` via `exec.Command`.

**Why not download directly via `kkdai/youtube`:** YouTube's audio stream URLs require PO token resolution and nsig signature deciphering, which change frequently. `yt-dlp` is maintained by a large community specifically to track these countermeasures. Implementing this in Go would duplicate that work and lag behind YouTube's changes.

**Required for:** `download_audio: true` in config.

**Install:**
```bash
pip install yt-dlp
# or
brew install yt-dlp
```

### `ffmpeg`

Audio format conversion. When present, `yt-dlp` uses it to extract and convert downloaded audio to opus format (`-x --audio-format opus`). Without ffmpeg, audio is saved in its native container (typically webm/opus).

**Required for:** Opus conversion. Optional — greenclaw degrades gracefully without it.

### `faster-whisper`

CTranslate2-based reimplementation of OpenAI's Whisper for speech-to-text transcription. Used in `internal/transcriber/whisper.go` via `exec.Command`, following the same subprocess pattern as yt-dlp.

**Why this over alternatives:** See [ADR 002](adr/002-audio-transcription.md) for the full engine comparison (original Whisper, whisper.cpp, external APIs).

**Required for:** `transcribe_audio: true` in config.

**Install:**
```bash
pip install faster-whisper
```

### Tini

Docker init process (PID 1). Reaps zombie Chrome, yt-dlp, and faster-whisper processes that did not exit cleanly when greenclaw's main process terminates.

**Required for:** Docker only. Installed via `apt-get` in the Dockerfile.

---

## Go Modules (Indirect)

These are pulled in by direct dependencies. Listed for transparency; they are not used directly by greenclaw's own code.

| Module | Pulled in by | Purpose |
| ------ | ------------ | ------- |
| `golang.org/x/net` | goquery | `net/html` tokenizer that backs goquery's parser |
| `golang.org/x/text` | goquery, x/net | Unicode text transformations |
| `github.com/andybalholm/cascadia` | goquery | CSS selector engine |
| `github.com/dop251/goja` | go-rod | JavaScript engine used by go-rod for sourcemap support |
| `github.com/dlclark/regexp2` | goja | .NET-compatible regex for goja |
| `github.com/go-sourcemap/sourcemap` | goja | Sourcemap parsing |
| `github.com/bitly/go-simplejson` | kkdai/youtube | JSON helper used in YouTube client |
| `github.com/ysmood/fetchup` | go-rod | Chrome binary downloader |
| `github.com/ysmood/leakless` | go-rod | Ensures child processes are killed when parent exits |
| `github.com/ysmood/goob` | go-rod | Observable event bus used internally |
| `github.com/ysmood/gson` | go-rod | JSON helper used internally |
| `github.com/google/pprof` | go-rod | Performance profiling (test dependency) |
