# Stage 1: Build Go binary
FROM golang:1.26.1-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o greenclaw -ldflags="-s -w" .

# Stage 2: Install Python dependencies
FROM python:3.12-slim-bookworm AS pip-deps

RUN pip install --no-cache-dir yt-dlp whisper-ctranslate2

# Stage 3: Runtime
FROM python:3.12-slim-bookworm

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ffmpeg \
        tini \
        chromium \
    && rm -rf /var/lib/apt/lists/*

# Copy Python packages from pip stage
COPY --from=pip-deps /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=pip-deps /usr/local/bin/yt-dlp /usr/local/bin/yt-dlp
COPY --from=pip-deps /usr/local/bin/whisper-ctranslate2 /usr/local/bin/whisper-ctranslate2
RUN ln -s /usr/local/bin/whisper-ctranslate2 /usr/local/bin/faster-whisper

# Copy Go binary
COPY --from=builder /src/greenclaw /app/greenclaw

WORKDIR /app
COPY config.yaml .

ENV CHROMIUM_PATH=/usr/bin/chromium

EXPOSE 8080

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["./greenclaw"]
