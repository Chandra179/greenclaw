# Web Scraper Architecture

### Overview

This scraper uses a **detect-first, escalate-later** strategy. Before launching a browser, it inspects the content type and attempts the lightest possible extraction method. A full headless browser is only used as a last resort when simpler approaches are blocked.

***

### Architecture

#### 1. Content-Type Router

Every URL first goes through a `HEAD` request to inspect the `Content-Type` header before any parsing or browser work begins.

```
Incoming URL
    │
    ▼
HEAD request → inspect Content-Type
    │
    ├── application/pdf, image/*  →  Stream to disk (io.Copy, no parsing)
    ├── application/json, text/xml →  HTTP GET → Unmarshal
    └── text/html                  →  HTML pipeline (see below)
```

**Libraries**: `net/http` for the HEAD request, standard `mime` package for type detection.

***

#### 2. HTML Pipeline

For `text/html` responses, the scraper attempts the lightest extraction method first and escalates only if needed.

```
text/html
    │
    ▼
[Stage 1] Plain HTTP + goquery
    │  net/http + PuerkitoBio/goquery
    │
    ├── Data extracted? ──Yes──► Store result
    │
    └── No / Blocked
            │
            ▼
    [Stage 2] Headless browser
         Go-Rod / Browserless + stealth plugin
            │
            ▼
    Anti-bot config applied
            │
            ▼
    Extract → Recycle browser → Store result
```

***

#### 3. Anti-Bot Configuration

Applied only when a headless browser is launched (Stage 2 above).

**Stealth spoofing**

* Set `navigator.webdriver = false`
* Spoof screen resolution (e.g. `1920x1080`)
* Fake battery level, installed fonts, canvas fingerprint
* Realistic User-Agent (no `HeadlessChrome` string)

**Human behaviour simulation**

* Random delays between page actions
* Simulated mouse curves and natural scroll patterns
* Realistic typing cadence for form inputs

**Browser flags (VPS-safe)**

```
--disable-gpu
--disable-dev-shm-usage
--no-sandbox
```

***

#### 4. Resource Management

**Concurrency limits (single VPS)**

| RAM   | Safe concurrent browser sessions |
| ----- | -------------------------------- |
| 2 GB  | 5–8                              |
| 4 GB  | 10–15                            |
| 16 GB | 50–80                            |

Each tab takes \~100–300 MB RAM. Plain HTTP sessions have negligible overhead and can run at much higher concurrency.

**Browser recycling (the "Rule of 100")**

Close and restart the entire browser instance every **100–200 pages** to prevent memory leak accumulation.

```go
defer browser.Close()
```

Use **Tini** as the Docker `init` process to reap zombie Chrome processes that did not shut down cleanly.

**Request interception**

Block unnecessary resource types to reduce RAM per page:

| Resource                | Block? | Reason                           |
| ----------------------- | ------ | -------------------------------- |
| Images (jpg, png, webp) | Yes    | Largest RAM impact               |
| Fonts (woff, ttf)       | Yes    | No data value                    |
| CSS                     | Yes\*  | Skip unless JS checks visibility |
| Ads / trackers          | Yes    | Use domain blocklist             |
| Main document + JS      | No     | Required for rendering           |

***

#### 5. Browser Engine Notes

The scraper relies on **Blink** (via Chrome/Chromium) as the rendering engine, which is required to pass browser fingerprint checks that V8 alone cannot satisfy:

* **Canvas rendering** — Blink draws the fingerprint image; V8 cannot draw at all.
* **WebRTC / network stack** — real browser behaviour is expected.
* **Event listeners** — sites check for `window.chrome` and similar browser globals.

Running in plain headless mode is detectable. The stealth layer (see section 3) patches the most common footprints before any page interaction.

***

### Decision Flow Summary

```
URL received
  │
  ├─ PDF / Image  ──────────────────────────► Stream to disk
  │
  ├─ JSON / XML   ──────────────────────────► HTTP GET + Unmarshal
  │
  └─ HTML
       │
       ├─ Plain HTTP succeeds  ────────────► Store result
       │
       └─ Blocked / JS-required
            │
            └─ Headless browser + stealth ─► Extract → Recycle → Store
```

***

### Tech Stack

* Use in memory data store first for prototype

| Concern              | Library / Tool                 |
| -------------------- | ------------------------------ |
| HTTP client          | `net/http`                     |
| HTML parsing         | `PuerkitoBio/goquery`          |
| Headless browser     | Go-Rod or Browserless (Docker) |
| Stealth              | Go-Rod stealth plugin          |
| Docker init          | Tini                           |
| PDF / binary streams | `io.Copy`                      |