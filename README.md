# greenclaw

A detect-first, escalate-later web scraper written in Go. It attempts the lightest possible extraction method for each URL and only launches a headless browser when simpler approaches fail.

---

## Quick Start

**Run with Docker:**

```bash
uvicorn main:app --host 0.0.0.0 --port 9000
make docker-build
make docker-run
curl -X POST http://localhost:8080/extract \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://www.youtube.com/watch?v=bufMa2Oscok"}'
```

---

## Documentation

| Doc | Description |
| --- | ----------- |
| [docs/architecture.md](docs/architecture.md) | System architecture, data flow, concurrency model |
| [docs/youtube.md](docs/youtube.md) | YouTube feature reference: metadata, transcripts, audio, subtitles |
| [docs/dependencies.md](docs/dependencies.md) | All dependencies and why each was chosen |
| [docs/adr/index.md](docs/adr/index.md) | Architecture Decision Records index |

---

## Installation
```
sudo apt-get install libcublas12
```