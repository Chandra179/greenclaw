# ADR 001: Docker Containerization

## Status

Accepted

## Date

2026-03-20

## Context

greenclaw requires three separate runtime environments to function fully:

- **Go** — the core scraper binary
- **Python + pip** — yt-dlp for YouTube audio downloads, and (planned) faster-whisper for transcription
- **System packages** — ffmpeg for audio conversion, Chromium for browser escalation

Setting this up locally is manual and error-prone. Users must install Go, pip-install yt-dlp, ensure ffmpeg is in PATH, and optionally install Chromium. The dependency surface will grow with the addition of the transcriber module (ADR 002).

## Decision

Containerize greenclaw using a **multi-stage Dockerfile** with a **Python slim runtime base image**.

### Build stages

```
Stage 1: golang:1.24-bookworm (builder)
  → go build -o greenclaw -ldflags="-s -w" .
  → Produces a statically-linked Go binary

Stage 2: python:3.12-slim-bookworm (pip dependencies)
  → pip install --no-cache-dir yt-dlp faster-whisper
  → Isolated layer for Python packages

Stage 3: python:3.12-slim-bookworm (runtime)
  → COPY Go binary from stage 1
  → COPY Python site-packages from stage 2
  → apt-get install: ffmpeg, tini, chromium
  → ENTRYPOINT ["/usr/bin/tini", "--"]
  → CMD ["./greenclaw"]
```

### Container orchestration

A single `docker-compose.yaml` with one service:

```yaml
services:
  greenclaw:
    build: .
    volumes:
      - ./downloads:/app/downloads
      - whisper-models:/models/whisper
volumes:
  whisper-models:
```

Whisper models are volume-mounted rather than baked into the image. This keeps the image at ~800 MB instead of 2.5 GB+ and allows model swapping without rebuilds.

### Init process

Tini is used as PID 1 to properly reap zombie processes. This is necessary because greenclaw spawns subprocesses (yt-dlp, faster-whisper, Chromium) that can leave zombie children if the main process doesn't wait on them.

## Alternatives Considered

### Alpine-based image

- **Pros:** Smaller base image (~5 MB vs ~50 MB)
- **Cons:** Uses musl libc. faster-whisper depends on CTranslate2 which links against glibc. Building CTranslate2 against musl is unsupported and fragile. Chromium on Alpine also requires additional workarounds.
- **Verdict:** Rejected. The glibc requirement from faster-whisper and Chromium makes Alpine impractical.

### Distroless image

- **Pros:** Minimal attack surface, no shell
- **Cons:** greenclaw's subprocess pattern (yt-dlp, faster-whisper) requires a shell and standard process execution environment. Distroless images lack `/bin/sh`, making `exec.Command` unreliable for Python-based tools.
- **Verdict:** Rejected. Subprocess execution is a core pattern in this codebase.

### Separate containers (Go + Python sidecar)

- **Pros:** Cleaner separation of concerns
- **Cons:** Adds IPC complexity (gRPC/HTTP between containers), filesystem sharing for audio files, and orchestration overhead. All tools are subprocesses in a single pipeline — they don't run as long-lived services.
- **Verdict:** Rejected. Single-container is simpler and matches the subprocess execution model.

### GPU support in v1

- **Pros:** Dramatically faster whisper transcription (50x+)
- **Cons:** Requires nvidia-container-toolkit, CUDA base image (~4 GB+), and NVIDIA GPU on host. Not all users have GPUs.
- **Verdict:** Deferred. CPU-only for v1. GPU support can be added later via `docker compose --profile gpu` with an nvidia runtime override.

## Consequences

- **Positive:** Single `docker compose up` replaces manual multi-tool installation. Reproducible builds across machines. Volume-mounted models keep image size reasonable.
- **Negative:** ~800 MB image size (Chromium is the largest contributor). Users without Docker need the existing manual setup path. CPU-only whisper will be slow for long audio files.
- **Risks:** Chromium inside Docker can be flaky with certain seccomp profiles — may need `--cap-add=SYS_ADMIN` or `--security-opt seccomp=unconfined` for some hosts.

## Implementation

Implemented in: `Dockerfile`, `docker-compose.yaml`, `.dockerignore`, `Makefile` (`docker-build`, `docker-run` targets).
