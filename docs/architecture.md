# Architecture

greenclaw uses a **detect-first, escalate-later** strategy. Before launching a browser, it inspects the content type and attempts the lightest possible extraction method. YouTube URLs bypass the content-type router entirely and use a dedicated extraction path.

---

## Data Flow

```
Incoming URL
    │
    ├── router.IsYouTube? ──Yes──► YouTube service (no browser)
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