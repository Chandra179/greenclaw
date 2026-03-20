# YouTube

greenclaw detects YouTube URLs before the standard content-type router runs. When a YouTube URL is recognized, it bypasses the HTML pipeline entirely and uses a dedicated extraction path. No headless browser is launched for YouTube URLs.

---

## URL Detection

`router.IsYouTube()` matches against known YouTube hosts (`youtube.com`, `www.youtube.com`, `m.youtube.com`, `youtu.be`) and classifies the URL into one of three types:

| URL Type | Patterns |
| -------- | -------- |
| **Video** | `/watch?v=ID`, `/shorts/ID`, `/embed/ID`, `youtu.be/ID` |
| **Playlist** | `/playlist?list=ID` |
| **Channel** | `/channel/UC...`, `/@handle` |

---

## Data Flow

```
URL → router.IsYouTube
  │
  ├── Video
  │     ├── GetVideoMetadata (title, duration, views, channel, captions)
  │     ├── GetAllTranscripts / GetTranscript (if extract_transcripts enabled)
  │     ├── DownloadAudio via yt-dlp (if download_audio enabled)
  │     │     └── Transcribe via faster-whisper (if transcribe_audio enabled)
  │     └── ExportSubtitles (if export_subtitles enabled)
  │
  ├── Playlist
  │     └── GetPlaylistItems (video IDs, titles, indices)
  │
  └── Channel
        └── Basic info only (channel ID / handle captured)
```

---

## Features

### Video Metadata

Fetches via `youtube.Client.GetVideoMetadata`:

- Video ID, title, description
- Duration, view count, upload date
- Channel name and ID
- Available caption tracks (language, language code, auto-generated flag)

### Transcript Extraction

Fetches YouTube's timed text XML format and converts to plain text.

- **Single language**: `GetTranscript(ctx, video, "en")` returns one caption track
- **All languages**: `GetAllTranscripts(ctx, video)` fetches every available track
- Language filtering via `transcript_langs` config (empty = all languages)
- HTML entities are unescaped during parsing

### Audio Download

`DownloadAudio` shells out to `yt-dlp` for reliable audio extraction. YouTube's stream URLs require anti-bot countermeasures (PO tokens, nsig deciphering) that `yt-dlp` actively maintains, making it far more reliable than direct HTTP stream downloads.

1. Requires `yt-dlp` in PATH
2. Selects the best audio-only stream (`-f bestaudio`)
3. When `ffmpeg` is available: extracts and converts to opus (`-x --audio-format opus`)
4. Without `ffmpeg`: downloads in the native format (typically webm/opus)
5. Output: `<audioOutputDir>/<videoID>.<ext>`

### Audio Transcription

When YouTube captions are unavailable, greenclaw transcribes downloaded audio using [faster-whisper](https://github.com/SYSTRAN/faster-whisper). Requires `faster-whisper` in PATH and an audio file to have been downloaded first.

- Runs only when `transcribe_audio: true` and `download_audio: true`
- YouTube captions take priority; whisper transcription is the fallback for `Result.Text`
- Default model: `base` (74 MB, ~16× realtime on a 4-core CPU)

See [ADR 002](adr/002-audio-transcription.md) for engine selection rationale.

### Subtitle Export

`ExportSubtitles` converts caption tracks to standard subtitle formats:

| Format | Extension | Notes |
| ------ | --------- | ----- |
| SRT | `.srt` | SubRip, `HH:MM:SS,mmm` timing |
| VTT | `.vtt` | WebVTT, `HH:MM:SS.mmm` timing |
| TTML | `.ttml` | XML-based, W3C standard |

Output: `<subtitleOutputDir>/<videoID>.<lang>.<format>`

### Playlist Support

`GetPlaylistItems` extracts all videos in a playlist: video ID, title, and index for each entry.

### Channel Support

Minimal — captures channel ID or handle. Does not resolve the uploads playlist.

---

## Configuration

YouTube settings live under the `youtube` key in `config.yaml`:

```yaml
youtube:
  extract_transcripts: true         # Fetch captions (default: true)
  transcript_langs: [en, es]        # Filter languages (default: all)
  download_audio: false             # Download audio files (default: false)
  audio_output_dir: downloads/audio
  transcribe_audio: false           # Transcribe audio when no captions (default: false)
  export_subtitles: false           # Export subtitle files (default: false)
  subtitle_formats: [srt]           # Formats: srt, vtt, ttml
  subtitle_output_dir: downloads/subtitles

transcriber:
  model: base                       # Whisper model: tiny, base, small, medium, large-v3
  model_dir: /models/whisper        # Directory for model files
  language: ""                      # ISO 639-1 code, empty = auto-detect
```

---

## CLI Flags

```bash
go run main.go --youtube-audio https://www.youtube.com/watch?v=ID
go run main.go --youtube-subtitles srt,vtt https://www.youtube.com/watch?v=ID
go run main.go --youtube-langs en,es https://www.youtube.com/watch?v=ID
go run main.go --youtube-transcribe https://www.youtube.com/watch?v=ID
```

---

## Package Layout

| File | Responsibility |
| ---- | -------------- |
| `internal/router/youtube.go` | URL detection and classification |
| `internal/store/youtube.go` | Data structures (`YouTubeData`, `CaptionTrack`, `PlaylistItem`) |
| `internal/youtube/client.go` | Video metadata and playlist extraction |
| `internal/youtube/transcript.go` | Caption/transcript fetching and parsing |
| `internal/youtube/audio.go` | yt-dlp audio download orchestration |
| `internal/youtube/subtitle.go` | Subtitle format conversion and export |
| `internal/scraper/youtube.go` | Orchestration of all YouTube extraction |
| `internal/transcriber/transcriber.go` | Interface, `Options`, and `Result` types |
| `internal/transcriber/whisper.go` | faster-whisper subprocess wrapper |
| `internal/config/config.go` | `YouTubeConfig` + `TranscriberConfig` structs |
