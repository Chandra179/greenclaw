# Architecture

greenclaw uses a **detect-first, escalate-later** strategy. Before launching a browser, it inspects the content type and attempts the lightest possible extraction method. YouTube URLs bypass the content-type router entirely and use a dedicated multi-stage pipeline.

---

## Data Flow

```
Incoming URL
    │
    ├── router.IsYouTube? ──Yes──► youtube.Process
    │                                   │
    │                              ┌────┴────────────────────────────┐
    │                           Video                          Playlist / Channel
    │                              │                                  │
    │                         5-stage pipeline                   metadata only
    │                         ① metadata (seq)
    │                         ② transcripts + audio (parallel)
    │                         ③ merge (seq)
    │                         ④ transcription (seq)
    │                         ⑤ LLM processing (seq, streaming)
    │
    └── No
          │
          ▼
     HEAD request → router.Classify → ContentType
          │
          ├── Binary (pdf, image, …) →  fetcher.DownloadBinary (io.Copy)
          ├── JSON / XML             →  fetcher.FetchJSON / FetchXML
          └── HTML                  →  fetcher.FetchHTML (goquery)
                    │
                    ├── extracted? ──Yes──► store.Result
                    │
                    └── ErrNeedsEscalation
                              │
                              ▼
                       browser.Pool.FetchPage
                       (go-rod + stealth plugin)
                              │
                              ▼
                       extract → recycle → store.Result
```

---

## Key Packages

| Package | Role |
|---|---|
| `cmd/app` | Entrypoint — loads config, starts HTTP server |
| `internal/server` | Gin router; `POST /extract` (sync) and `POST /extract/stream` (SSE) |
| `internal/pipeline` | Top-level orchestrator; routes URLs, manages retries and HTTP semaphore |
| `internal/router` | HEAD request to detect `ContentType`; parses YouTube URLs into type + ID |
| `internal/scraper` | Coordinates `fetcher` and `browser`; holds browser semaphore |
| `internal/fetcher` | Stage-1 HTTP fetch (goquery for HTML, raw unmarshal for JSON/XML, io.Copy for binaries); returns `ErrNeedsEscalation` on JS-only pages |
| `internal/browser` | go-rod pool with stealth plugin; recycles browser every N pages; blocks images/fonts/CSS/ads |
| `internal/youtube` | YouTube extraction: metadata, captions, audio via yt-dlp, transcription, LLM |
| `internal/llm` | Ollama client; chunked processing with two strategies (refine for summaries, map-reduce for takeaways); file-based cache |
| `internal/store` | Thread-safe in-memory store; `Result` is the universal output type |
| `internal/config` | `Config` struct with defaults; loaded from `config.yaml` |

---

## Concurrency Model

Two semaphores decouple HTTP and browser concurrency:

- **`httpSem`** (pipeline) — limits overall HTTP concurrency across all URL processing.
- **`browserSem`** (scraper) — limits concurrent headless browser instances.

The HTTP semaphore is released before the browser semaphore is acquired during escalation to prevent deadlocks.

---

## LLM Processing

Large transcripts are split by a `RecursiveChunker` (paragraph → sentence → word boundaries) with configurable token overlap. Two strategies are supported:

- **Refine** (`StyleSummary`) — iterative: each chunk refines the running summary.
- **Map-Reduce** (`StyleTakeaways`) — parallel map (extract key points per chunk), then reduce (deduplicate and consolidate).

Results are cached to disk keyed by `(cacheKey, style, model, numCtx)`.
