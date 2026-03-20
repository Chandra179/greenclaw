# ADR 003: Web Scraping Strategy

## Status

Accepted

## Date

2026-03-20

## Context

greenclaw needs to reliably extract content from URLs that vary widely in how they serve data:

- Static HTML pages that return full content in the initial HTTP response
- JS-rendered SPAs that require a real browser to execute JavaScript before content appears
- APIs that return JSON or XML directly
- Binary assets (PDFs, images) that should be streamed to disk unchanged
- YouTube URLs that have a dedicated, browser-free extraction path

A single strategy (always use a browser, or always use plain HTTP) wastes resources or fails to work across this full range.

## Decision

Use a **detect-first, escalate-later** strategy with four distinct content-type paths and a YouTube early exit:

```
URL
  ├── IsYouTube?         → YouTube pipeline (kkdai/youtube + yt-dlp)
  ├── PDF / image        → io.Copy to disk (no parsing)
  ├── JSON / XML         → HTTP GET + Unmarshal
  └── HTML
        ├── Stage 1: plain HTTP + goquery
        └── Stage 2 (on ErrNeedsEscalation): go-rod + stealth
```

The browser is launched only when Stage 1 fails — either because the server actively blocks scrapers or because the content requires JavaScript to render.

### Why detect via HEAD request

A HEAD request returns the `Content-Type` header without transferring the body. This means binary assets (large PDFs, images) never touch the HTML parser, and JSON/XML responses never touch goquery. The cost is one extra round-trip, which is negligible compared to the cost of parsing the wrong content type.

### Why goquery for Stage 1

- Pure Go, no subprocess overhead
- CSS selector API is expressive enough for most static HTML
- Fast: no JavaScript engine, no browser process, no CDP handshake
- Fails fast — returns `ErrNeedsEscalation` when the response is a bot-detection page or a loading spinner

### Why go-rod for Stage 2

- Pure Go (no Node.js runtime required), consistent with the static-binary philosophy
- Full Chrome DevTools Protocol support — runs real Chromium, which passes all browser fingerprint checks that a JavaScript engine alone cannot satisfy
- Built-in request interception to block images, fonts, and CSS, reducing RAM per tab
- `go-rod/stealth` companion patches fingerprint vectors before any page load

### Why not Playwright or Puppeteer

Both require a Node.js runtime. Adding Node.js to the Docker image would increase size significantly and introduce a second runtime ecosystem. go-rod achieves the same CDP control in pure Go.

### Why not always use the browser

Chrome tabs consume 100–300 MB RAM each. At 5 concurrent browser sessions (the default), that's 500 MB–1.5 GB dedicated to browser alone on a typical VPS. Plain HTTP sessions are negligible. Running the browser for every URL — including static HTML pages and JSON APIs — would hit memory limits quickly and add unnecessary latency.

### YouTube as an early exit

YouTube's HTML is JS-rendered and protected by bot detection. Even Stage 2 (browser + stealth) would require heavy ongoing maintenance to stay functional as YouTube updates its anti-bot measures. `yt-dlp` and `kkdai/youtube` are maintained by dedicated communities tracking exactly these changes. Delegating YouTube to those tools is more reliable than fighting YouTube's bot detection in a general-purpose browser pipeline.

## Alternatives Considered

### Always use headless browser

- **Pros:** No two-stage logic; simpler code
- **Cons:** 10–20× higher resource consumption for sites that don't need it. Browser pool exhaustion becomes the bottleneck for bulk scraping.
- **Verdict:** Rejected. The vast majority of URLs serve static HTML or JSON — wasting a browser session on them is inefficient.

### Playwright (via `playwright-go`)

- **Pros:** Mature ecosystem, strong community, cross-browser support
- **Cons:** Requires downloading and managing a Node.js runtime inside Docker. The `playwright-go` bindings are community-maintained and lag behind the main project.
- **Verdict:** Rejected. go-rod provides equivalent CDP-level control in pure Go without the Node.js dependency.

### Splash (Scrapy-compatible JS rendering service)

- **Pros:** Decouples browser from scraper process; horizontal scaling
- **Cons:** Another service to operate and keep running. Adds HTTP round-trip latency per page. Requires a separate Docker container.
- **Verdict:** Rejected. The overhead of a sidecar service is not justified for greenclaw's single-binary model.

### Chromedp

- **Pros:** Pure Go CDP client, part of the Go standard tooling ecosystem
- **Cons:** Lower-level API than go-rod — no built-in browser lifecycle management, pool management, or request interception helpers. More boilerplate for the same outcome.
- **Verdict:** Not selected. go-rod's higher-level API reduces implementation complexity for this use case.

## Consequences

- **Positive:** Low resource usage for the common case (static HTML, JSON). Browser is reserved for sites that genuinely require it. YouTube extraction is reliable without fighting bot detection.
- **Negative:** Two-stage logic means some pages are fetched twice (HEAD + Stage 1 GET, then Stage 2 if escalation is needed). This is an acceptable cost — escalation happens rarely on most URL sets.
- **Risks:** Stage 1 failure detection relies on goquery finding meaningful content. If a site returns a 200 OK with a loading spinner and no error, Stage 1 may incorrectly report success. Mitigation: caller-supplied content selectors and minimum-content checks.
