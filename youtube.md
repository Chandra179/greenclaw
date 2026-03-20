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
  │     ├── DownloadAudio (if download_audio enabled)
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

`DownloadAudio` selects the best audio-only stream:

1. Prefers `opus/webm` (highest bitrate)
2. Falls back to highest bitrate `m4a/mp4`
3. Streams to disk via `io.Copy`
4. Output: `<outputDir>/<videoID>.webm` or `.m4a`

#### Subtitle Export

`ExportSubtitles` converts caption tracks to standard subtitle formats:

| Format | Extension | Notes                          |
| ------ | --------- | ------------------------------ |
| SRT    | `.srt`    | SubRip, `HH:MM:SS,mmm` timing |
| VTT    | `.vtt`    | WebVTT, `HH:MM:SS.mmm` timing |
| TTML   | `.ttml`   | XML-based, W3C standard        |

Output: `<outputDir>/<videoID>.<lang>.<format>`

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
  export_subtitles: false           # Export subtitle files (default: false)
  subtitle_formats: [srt]           # Formats: srt, vtt, ttml
  subtitle_output_dir: downloads/subtitles
```

#### CLI Flags

```bash
go run main.go --youtube-audio https://youtube.com/watch?v=ID
go run main.go --youtube-subtitles srt,vtt https://youtube.com/watch?v=ID
go run main.go --youtube-langs en,es https://youtube.com/watch?v=ID
```

---

### Package Layout

| File                          | Responsibility                          |
| ----------------------------- | --------------------------------------- |
| `internal/router/youtube.go`  | URL detection and classification        |
| `internal/store/youtube.go`   | Data structures (`YouTubeData`, etc.)   |
| `internal/youtube/client.go`  | Video metadata and playlist extraction  |
| `internal/youtube/transcript.go` | Caption/transcript fetching and parsing |
| `internal/youtube/audio.go`   | Audio stream selection and download     |
| `internal/youtube/subtitle.go`| Subtitle format conversion and export   |
| `internal/scraper/youtube.go` | Orchestration of all YouTube extraction |
| `internal/config/config.go`   | `YouTubeConfig` struct and defaults     |

---

### Dependencies

- **`github.com/kkdai/youtube/v2`** — Video metadata, stream URLs, playlist data, caption track URLs
- **`encoding/xml`** — Parsing YouTube's timed text XML caption format
- **`net/http`** — Fetching caption track content
