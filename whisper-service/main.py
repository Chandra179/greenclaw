"""Whisper transcription HTTP service.

Wraps faster-whisper in a FastAPI app so the Go scraper can offload
transcription to a host machine with GPU access.

    WHISPER_MODEL        model size (default: medium)
    WHISPER_DEVICE       cuda | cpu (default: cuda)
    WHISPER_COMPUTE_TYPE float16 | int8_float16 | int8 (default: int8_float16)
    WHISPER_MODEL_DIR    optional path to cached model files
"""

import logging
import os
import tempfile
import time
from contextlib import asynccontextmanager

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

from fastapi import FastAPI, File, Form, HTTPException, UploadFile
from faster_whisper import WhisperModel

# Changed defaults to be safer for 6GB VRAM out of the box
MODEL_SIZE = os.getenv("WHISPER_MODEL", "medium") 
DEVICE = os.getenv("WHISPER_DEVICE", "cuda")
COMPUTE_TYPE = os.getenv("WHISPER_COMPUTE_TYPE", "int8_float16")
MODEL_DIR = os.getenv("WHISPER_MODEL_DIR") or None

model: WhisperModel | None = None

@asynccontextmanager
async def lifespan(app: FastAPI):
    global model
    model = WhisperModel(
        MODEL_SIZE,
        device=DEVICE,
        compute_type=COMPUTE_TYPE,
        download_root=MODEL_DIR,
        # Restrict workers so we don't spawn multiple VRAM-hungry threads
        num_workers=1 
    )
    yield
    model = None

app = FastAPI(title="whisper-service", lifespan=lifespan)

@app.get("/health")
async def health():
    return {
        "status": "ok",
        "model": MODEL_SIZE,
        "device": DEVICE,
        "compute_type": COMPUTE_TYPE,
    }

@app.post("/transcribe")
async def transcribe(
    file: UploadFile = File(...),
    language: str = Form(""),
):
    if model is None:
        raise HTTPException(status_code=503, detail="model not loaded")

    suffix = os.path.splitext(file.filename or "audio.wav")[1]
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp_path = tmp.name
        while chunk := await file.read(1024 * 1024):
            tmp.write(chunk)

    try:
        lang = language if language else None

        # Lowering beam_size from default (5) to 2 or 1 saves VRAM during execution
        # with a very negligible drop in transcription accuracy.
        t0 = time.monotonic()
        segments, info = model.transcribe(
            tmp_path,
            language=lang,
            beam_size=2
        )

        parts = []
        duration = 0.0
        for seg in segments:
            parts.append(seg.text.strip())
            duration = seg.end

        elapsed = time.monotonic() - t0
        logger.info(
            "transcribed %.1fs of audio in %.2fs (%.1fx realtime)",
            duration, elapsed, duration / elapsed if elapsed > 0 else 0,
        )

        return {
            "text": " ".join(parts),
            "language": info.language,
            "duration": duration,
        }
    finally:
        os.unlink(tmp_path)