# Architecture

greenclaw uses a **detect-first, escalate-later** strategy. Before launching a browser, it inspects the content type and attempts the lightest possible extraction method. YouTube URLs bypass the content-type router entirely and use a dedicated extraction path.

---

## Data Flow

```
Incoming URL
    │
    ├── router.IsYouTube? ──Yes──► YouTube pipeline (no browser)
    │
    └── No
          │
          ▼
     HEAD request → inspect Content-Type
          │
          ├── application/pdf, image/*  →  Stream to disk (io.Copy)
          ├── application/json, text/xml →  HTTP GET + Unmarshal
          └── text/html                  →  HTML pipeline
                    │
                    ▼
             [Stage 1] Plain HTTP + goquery
                    │
                    ├── Data extracted? ──Yes──► Store result
                    │
                    └── ErrNeedsEscalation
                              │
                              ▼
                       [Stage 2] Headless browser
                        go-rod + stealth plugin
                              │
                              ▼
                       Extract → Recycle → Store
```

---

## Packages

| Package | Responsibility |
| ------- | -------------- |
| `internal/router` | HEAD request; classifies URLs into `store.ContentType` constants. `IsYouTube()` intercepts YouTube URLs before the HEAD request. |
| `internal/fetcher` | Stage 1 HTTP fetching. goquery for HTML, raw unmarshal for JSON/XML, `io.Copy` for binaries. Returns `ErrNeedsEscalation` when blocked or JS-required. |
| `internal/browser` | go-rod browser pool with stealth plugin. Recycles the browser every `RecycleAfter` pages (default 100). Blocks images, fonts, CSS, and ad domains via request interception. |
| `internal/youtube` | YouTube metadata, transcripts, audio download, subtitle export. |
| `internal/transcriber` | faster-whisper subprocess wrapper for speech-to-text transcription. |
| `internal/scraper` | Orchestrates concurrency (two semaphores: `httpSem`, `browserSem`), retry with exponential backoff, and context cancellation. |
| `internal/store` | Thread-safe in-memory result store. `Result` is the universal output type. |
| `internal/config` | `Config` struct and defaults. Loaded from `config.yaml`. |

---

## Concurrency Model

Two separate semaphores control HTTP and browser concurrency independently:

- **`httpSem`** — held during URL classification (HEAD) and Stage 1 fetch
- **`browserSem`** — acquired only on escalation, *after* `httpSem` is released

Releasing `httpSem` before acquiring `browserSem` prevents deadlock when all HTTP slots are occupied and escalation is needed.

Defaults: 20 HTTP concurrent / 5 browser concurrent. Configurable via `config.yaml`.

---

## Anti-Bot Configuration

Applied only in Stage 2 (headless browser).

**Stealth spoofing**

- `navigator.webdriver = false`
- Spoofed screen resolution (1920×1080)
- Faked battery level, installed fonts, canvas fingerprint
- Realistic User-Agent (no `HeadlessChrome` string)

**Human behaviour simulation**

- Random delays between page actions
- Simulated mouse curves and natural scroll patterns
- Realistic typing cadence for form inputs

**Browser flags (VPS-safe)**

```
--disable-gpu
--disable-dev-shm-usage
--no-sandbox
```

---

## Resource Management

**Browser recycling**

Close and restart the browser instance every 100–200 pages (`RecycleAfter`, default 100) to prevent memory leak accumulation from Chrome internals.

**Request interception**

| Resource | Blocked | Reason |
| -------- | ------- | ------ |
| Images (jpg, png, webp) | Yes | Largest RAM impact |
| Fonts (woff, ttf) | Yes | No data value |
| CSS | Yes* | Skip unless JS checks visibility |
| Ads / trackers | Yes | Domain blocklist |
| Main document + JS | No | Required for rendering |

**Concurrency limits (reference)**

| RAM | Safe concurrent browser sessions |
| --- | -------------------------------- |
| 2 GB | 5–8 |
| 4 GB | 10–15 |
| 16 GB | 50–80 |

Plain HTTP sessions have negligible overhead and run at much higher concurrency.

---

## Browser Engine

greenclaw uses Blink (via Chromium) as the rendering engine. Plain headless mode is detectable; the stealth layer patches the most common fingerprints before any page interaction. See [ADR 003](adr/003-scraping-strategy.md) for the full decision rationale.

---

## Tech Stack

| Concern | Library / Tool |
| ------- | -------------- |
| HTTP client | `net/http` |
| HTML parsing (Stage 1) | `PuerkitoBio/goquery` |
| Headless browser (Stage 2) | `go-rod/rod` |
| Stealth patches | `go-rod/stealth` |
| YouTube extraction | `kkdai/youtube/v2` |
| Audio download | `yt-dlp` (external binary) |
| Speech-to-text | `faster-whisper` (external binary, optional) |
| Docker init | Tini |
| Config | `gopkg.in/yaml.v3` |
| Binary streams | `io.Copy` |

See [dependencies.md](dependencies.md) for rationale behind each dependency choice.
