# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build ./...          # build
go test ./...           # run all tests
go test ./internal/...  # run tests, specific package (e.g. ./internal/fetcher)
go run main.go <url>    # run with a URL
go run main.go --urls-file urls.txt  # run with a file of URLs
go run main.go --output json <url>   # JSON output
```

## Architecture

**greenclaw** is a detect-first, escalate-later web scraper. It attempts the lightest extraction method and only launches a headless browser as a last resort.

### Data flow

```
URL → router.Classify (HEAD) → content type → fetcher
  ContentBinary → fetcher.DownloadBinary (io.Copy to disk)
  ContentJSON   → fetcher.FetchJSON
  ContentXML    → fetcher.FetchXML
  ContentHTML   → fetcher.FetchHTML (goquery) → on ErrNeedsEscalation → browser.Pool.FetchPage (go-rod + stealth)
```

### Key packages

- **`internal/router`** — HEAD request to detect content type; returns a `store.ContentType` constant
- **`internal/fetcher`** — Stage 1 HTTP fetching (goquery for HTML, raw unmarshal for JSON/XML, io.Copy for binaries). Returns `ErrNeedsEscalation` when a plain HTTP fetch is blocked/insufficient.
- **`internal/browser`** — go-rod browser pool with stealth plugin; recycles the browser instance every N pages (`RecycleAfter`, default 100) to prevent memory leaks. Blocks images, fonts, CSS, ads via request interception.
- **`internal/store`** — in-memory thread-safe result store. `Result` struct is the universal output type.
- **`internal/scraper`** — orchestrates concurrency via two semaphore channels (`httpSem`, `browserSem`), retry with exponential backoff, and graceful context cancellation.
- **`internal/config`** — `Config` struct with defaults (20 HTTP / 5 browser concurrent sessions, 30s timeout, 3 retries).

### Concurrency model

Two separate semaphores control HTTP and browser concurrency independently. The HTTP semaphore is held during classification and Stage 1 fetch; it is released before the browser semaphore is acquired to avoid deadlocks during escalation.
