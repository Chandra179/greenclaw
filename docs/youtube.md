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
  │     ├── GetTranscript (if extract_transcripts enabled)
  │     ├── DownloadAudio via yt-dlp (if download_audio enabled)
  │     │     └── Transcribe via faster-whisper (if transcribe_audio enabled)
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

When YouTube captions are unavailable, greenclaw transcribes downloaded audio by sending it to the **whisper-service** — a separate FastAPI HTTP service wrapping [faster-whisper](https://github.com/SYSTRAN/faster-whisper). The service runs outside the Go container to allow GPU access.

- Runs only when `transcribe_audio: true` and `download_audio: true`
- YouTube captions take priority; whisper transcription is the fallback for `Result.Text`
- Requires `transcriber.endpoint` in config pointing to a running whisper-service instance (e.g. `http://host.docker.internal:9000`)
- The service defaults to the `medium` model on CUDA; model and device are configured via environment variables

See [ADR 002](adr/002-audio-transcription.md) for engine selection rationale.

### Playlist Support

`GetPlaylistItems` extracts all videos in a playlist: video ID, title, and index for each entry.

### Channel Support

Minimal — captures channel ID or handle. Does not resolve the uploads playlist.