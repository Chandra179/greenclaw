# LLM Package Architecture

`internal/llm` processes transcripts through a local Ollama instance. Long transcripts are split into chunks and routed to one of two multi-step strategies depending on the requested style.

---

## Data Flow

```
Request{Style, Title, Text, CacheKey, ProgressCh}
    │
    ├── CacheKey set? ──Yes──► ResultCache.Get ──hit──► Result
    │
    ├── StyleSummary    ──► processRefine (rolling-window)
    │                           │
    │                      chunk 1 → initial summary
    │                      chunk 2 → refine(summary, chunk 2)
    │                         ...
    │                      chunk N → refine(summary, chunk N)
    │                           │
    │                        Result
    │
    ├── StyleTakeaways  ──► processMapReduce (parallel map + reduce)
    │                           │
    │                      chunk 1 ─┐
    │                      chunk 2 ─┤ concurrent → key_points[]
    │                      chunk N ─┘
    │                           │
    │                         reduce → deduplicated takeaways
    │                           │
    │                        Result
    │
    └── default         ──► single callWithRetry → Result
    │
    └── CacheKey set? ──Yes──► ResultCache.Put
```

---

## Key Packages

| File | Role |
|---|---|
| `process.go` | `Client` interface; `Request` and `Result` types |
| `ollama.go` | Ollama backend — HTTP calls, retry, prompt builders |
| `strategy.go` | `processRefine` and `processMapReduce` implementations |
| `chunk.go` | `RecursiveChunker` — splits at paragraph → sentence → word boundaries |
| `cache.go` | Disk-based result cache keyed by SHA-256 of `(cacheKey, style, model, numCtx)` |
| `progress.go` | `ProgressEvent` emitted non-blocking on `Request.ProgressCh` |
| `schema.go` | JSON schemas for Ollama structured output per style |

---

## Chunking

`RecursiveChunker` mirrors LangChain's `RecursiveCharacterTextSplitter`. It finds the largest available semantic boundary within each window (paragraph → newline → sentence → word), then repeats a configurable overlap at the start of the next chunk to preserve context across boundaries.

Chunk size is derived from `numCtx`: `(numCtx − 1000) × 4` characters, reserving 1000 tokens for prompt overhead and using a 4 chars/token heuristic. Default overlap is 200 tokens.

---

## Progress Reporting

Events are sent on the optional `Request.ProgressCh` channel. Sends are non-blocking — events are silently dropped if the consumer is slow.

```
chunk_start  →  chunk_done     (repeated per chunk, map phase)
reduce_start →  reduce_done    (once, reduce phase only)
```
