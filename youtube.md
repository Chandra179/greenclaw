# YouTube Feature

### Overview

greenclaw detects YouTube URLs before the standard content-type router runs. When a YouTube URL is recognized, it bypasses the normal HTML pipeline and uses a dedicated extraction path via the `kkdai/youtube/v2` library. No browser is launched for YouTube URLs.

---

### URL Detection

YouTube URLs are identified by `router.IsYouTube()` which matches against known YouTube hosts (`youtube.com`, `www.youtube.com`, `m.youtube.com`, `youtu.be`) and classifies the URL into one of three types:

| URL Type   | Patterns                                                        |
| ---------- | --------------------------------------------------------------- |
| **Video**    | `/watch?v=ID`, `/shorts/ID`, `/embed/ID`, `youtu.be/ID`       |
| **Playlist** | `/playlist?list=ID`                                           |
| **Channel**  | `/channel/UC...`, `/@handle`                                  |

---

### Data Flow

```
URL → router.IsYouTube
  │
  ├── Video
  │     ├── GetVideoMetadata (title, duration, views, channel, captions)
  │     ├── GetTranscript / GetAllTranscripts (if extract_transcripts enabled)
  │     ├── DownloadAudio via yt-dlp (if download_audio enabled)
  │     └── ExportSubtitles (if export_subtitles enabled)
  │
  ├── Playlist
  │     └── GetPlaylistItems (video IDs, titles, indices)
  │
  └── Channel
        └── Basic info only (channel ID / handle captured)
```

---

### Features

#### Video Metadata

Fetches via `youtube.Client.GetVideoMetadata`:

- Video ID, title, description
- Duration, view count, upload date
- Channel name and ID
- Thumbnail URL (highest resolution available)
- Available caption tracks (language, language code, auto-generated flag)

#### Transcript Extraction

Fetches YouTube's timed text XML format and converts to plain text.

- **Single language**: `GetTranscript(ctx, video, "en")` returns one caption track
- **All languages**: `GetAllTranscripts(ctx, video)` fetches every available track
- Language filtering via `transcript_langs` config (empty = all languages)
- HTML entities are unescaped during parsing

#### Audio Download

`DownloadAudio` shells out to `yt-dlp` for reliable audio extraction. YouTube's stream URLs require anti-bot countermeasures (PO tokens, nsig deciphering) that `yt-dlp` actively maintains, making it far more reliable than direct HTTP stream downloads.

1. Requires `yt-dlp` installed and in PATH
2. Selects the best audio-only stream (`-f bestaudio`)
3. When `ffmpeg` is available: extracts and converts to opus (`-x --audio-format opus`)
4. Without `ffmpeg`: downloads in the native format (typically webm/opus)
5. Output: `<outputDir>/<videoID>.<ext>` (`.opus`, `.webm`, or `.m4a`)

**Installing yt-dlp:**
```bash
pip install yt-dlp        # via pip
brew install yt-dlp       # macOS
```

#### Subtitle Export

`ExportSubtitles` converts caption tracks to standard subtitle formats:

| Format | Extension | Notes                          |
| ------ | --------- | ------------------------------ |
| SRT    | `.srt`    | SubRip, `HH:MM:SS,mmm` timing |
| VTT    | `.vtt`    | WebVTT, `HH:MM:SS.mmm` timing |
| TTML   | `.ttml`   | XML-based, W3C standard        |

Output: `<outputDir>/<videoID>.<lang>.<format>`

#### Audio Transcription (Planned)

When YouTube captions are unavailable, greenclaw can transcribe downloaded audio files using [faster-whisper](https://github.com/SYSTRAN/faster-whisper), a CTranslate2-based reimplementation of OpenAI's Whisper.

**Pipeline:**

```
Audio download (yt-dlp)
  → audio file on disk (.opus/.webm/.m4a)
    → CheckWhisper() (verify faster-whisper in PATH)
      → Transcribe(ctx, audioPath, opts)
        → faster-whisper subprocess (model: base by default)
          → Result { Text, Language, Duration }
            → Stored in ytData.TranscriptFromAudio
```

**Relationship to captions:**

- Transcription runs **only when** `transcribe_audio: true` and an audio file was downloaded
- YouTube captions (from the API) take priority for `Result.Text`
- If no captions exist, `TranscriptFromAudio` is used as the fallback text
- The two sources are stored separately: `Captions[]` vs `TranscriptFromAudio`

**Engine selection rationale:**

| Engine | Speed (CPU) | Why not? |
|--------|-------------|----------|
| OpenAI Whisper | 10-50x realtime | Too slow on CPU, large PyTorch dependency |
| whisper.cpp | Very fast | Requires CGO, complicates static Go binary |
| **faster-whisper** | **4-8x faster than original** | **Chosen: pip-based, subprocess pattern matches yt-dlp** |
| External APIs | N/A | Cost, privacy, network dependency |

**Model defaults:**

The `base` model (74 MB) is the default — it transcribes at ~16x realtime on a 4-core CPU. A 10-minute video takes approximately 37 seconds. Configurable via `transcriber.model` in config or future CLI flags.

See [ADR 002](docs/adr/002-audio-transcription.md) for full design rationale.

---

#### Playlist Support

`GetPlaylistItems` extracts all videos in a playlist, returning video ID, title, and index for each entry.

#### Channel Support

Minimal — captures channel ID or handle. Does not resolve the uploads playlist.

---

### Configuration

YouTube settings live under the `youtube` key in `config.yaml`:

```yaml
youtube:
  extract_transcripts: true         # Fetch transcripts (default: true)
  transcript_langs: [en, es]        # Filter languages (default: all)
  download_audio: false             # Download audio files (default: false)
  audio_output_dir: downloads/audio # Audio save location
  transcribe_audio: false           # Transcribe audio when no captions (default: false)
  export_subtitles: false           # Export subtitle files (default: false)
  subtitle_formats: [srt]           # Formats: srt, vtt, ttml
  subtitle_output_dir: downloads/subtitles

transcriber:
  model: base                       # Whisper model: tiny, base, small, medium, large-v3
  model_dir: /models/whisper        # Directory for model files
  language: ""                      # ISO 639-1 code, empty for auto-detect
```

#### CLI Flags

```bash
go run main.go --youtube-audio https://www.youtube.com/watch?v=bufMa2Oscok
go run main.go --youtube-subtitles srt,vtt https://www.youtube.com/watch?v=bufMa2Oscok
go run main.go --youtube-langs en,es https://youtube.com/watch?v=ID
go run main.go --youtube-transcribe https://www.youtube.com/watch?v=ID  # transcribe audio (planned)
```

---

### Package Layout

| File                          | Responsibility                          |
| ----------------------------- | --------------------------------------- |
| `internal/router/youtube.go`  | URL detection and classification        |
| `internal/store/youtube.go`   | Data structures (`YouTubeData`, etc.)   |
| `internal/youtube/client.go`  | Video metadata and playlist extraction  |
| `internal/youtube/transcript.go` | Caption/transcript fetching and parsing |
| `internal/youtube/audio.go`   | yt-dlp audio download orchestration     |
| `internal/youtube/subtitle.go`| Subtitle format conversion and export   |
| `internal/scraper/youtube.go` | Orchestration of all YouTube extraction |
| `internal/transcriber/transcriber.go` | Transcription interface and types (planned) |
| `internal/transcriber/whisper.go` | faster-whisper subprocess wrapper (planned) |
| `internal/config/config.go`   | `YouTubeConfig` + `TranscriberConfig` structs |

---

### Dependencies

- **`github.com/kkdai/youtube/v2`** — Video metadata, playlist data, caption track URLs
- **`yt-dlp`** (external binary) — Audio stream download with anti-bot countermeasures
- **`ffmpeg`** (optional external binary) — Audio format conversion (opus)
- **`encoding/xml`** — Parsing YouTube's timed text XML caption format
- **`net/http`** — Fetching caption track content
- **`faster-whisper`** (planned, external binary) — Speech-to-text transcription via CTranslate2
