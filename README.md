# greenclaw

YouTube content extraction and knowledge graph pipeline.

## Goal

Extract transcripts from YouTube videos, store them in ArangoDB, then build per-category knowledge graphs by extracting entities and relationships from the transcript text using an LLM.

---

## Implementation Plan

### Phase 1 — Transcript Extraction (`POST /extract/youtube`)

**Goal:** Given a YouTube URL, produce a transcript and persist the video + transcript to ArangoDB.

**Steps:**

1. Parse the video ID from the URL.
2. Fetch video metadata (`youtube.Client.GetVideoMetadata`).
3. **If captions exist** → fetch transcript (`youtube.Client.GetTranscript`).
4. **If no captions** → download audio (`youtube.Client.DownloadAudio`) → call whisper-service (`transcribe.Client.Transcribe`) to get transcript text.
5. Store a `videos` vertex in ArangoDB via `graphdb.Client.UpsertVertex`:
   ```json
   {
     "_key": "<videoID>",
     "title": "...",
     "url": "...",
     "transcript": "...",
     "language": "...",
     "duration": 1234,
     "processed": false,
     "category": ""
   }
   ```

**Implement in:** `internal/service/extract_youtube.go`

---

### Phase 2 — Graph Building (`POST /extract/graph`)

**Goal:** Given a YouTube URL (video already extracted), run LLM-based entity/relationship extraction and populate the knowledge graph. Graph is scoped per category.

#### 2a. Category assignment

Before entity extraction, classify the video into one of the supported categories:

| Category | Description |
|---|---|
| `economy` | macroeconomics, finance, markets, trade |
| `technology` | software, hardware, AI, engineering |

The LLM assigns the category based on transcript content. Store it back on the `videos` vertex (`category` field).

#### 2b. Entity extraction

Prompt the LLM with the transcript (chunked if needed) to extract entities. Supported entity types:

| Type | Examples |
|---|---|
| `concept` | "machine learning", "inflation", "open source" |
| `person` | "Elon Musk", "Jerome Powell" |
| `organization` | "OpenAI", "Federal Reserve", "Apple" |
| `event` | "2008 financial crisis", "WWDC 2024" |

Each entity is stored as a vertex in the `entities` collection:
```json
{
  "_key": "<normalized-slug>",
  "name": "OpenAI",
  "type": "organization",
  "category": "technology"
}
```

Entity normalization: lowercase, trim whitespace, deduplicate by slug key before upsert.

#### 2c. Relationship extraction

Two relationship types, stored as edges:

| Edge collection | Meaning | Direction |
|---|---|---|
| `related_to` | entity is related to another entity | `entities/<key>` → `entities/<key>` |

`related_to` edges are created when the LLM identifies co-occurrence or explicit semantic relationships between entities within the same category graph.

#### 2d. Graph scope

Graphs are **per-category**, not global. The named graph in ArangoDB (`knowledge`) spans all categories, but queries should filter by `category` on vertex documents to stay within a category boundary.

**Implement in:** `internal/service/build_graph.go`

---

### Phase 3 — LLM Prompts

Two prompts needed (implement in `pkg/llm` or a new `pkg/prompt` package):

1. **Classify** — input: transcript → output: `{"category": "technology"}`
2. **Extract** — input: transcript chunk → output:
   ```json
   {
     "entities": [
       {"name": "OpenAI", "type": "organization"},
       {"name": "GPT-4", "type": "concept"}
     ],
     "relationships": [
       {"from": "GPT-4", "to": "OpenAI", "type": "related_to"}
     ]
   }
   ```

Use `pkg/chunker.RecursiveChunker` to split long transcripts before extraction. Merge and deduplicate entities across chunks before writing to ArangoDB.

---

### Data model summary

```
videos (vertex)
  _key        videoID
  title       string
  url         string
  transcript  string
  language    string
  duration    int
  category    string   ← "economy" | "technology"
  processed   bool

entities (vertex)
  _key        slug (e.g. "open-ai")
  name        string
  type        string   ← "concept" | "person" | "organization" | "event"
  category    string

related_to (edge) entities/* → entities/*
```

---

### Current state

| Component | Status |
|---|---|
| `pkg/youtube` | Done — caption fetch + yt-dlp audio download |
| `pkg/transcribe` | Done — whisper-service HTTP client |
| `pkg/llm` | Done — Ollama client + chunker |
| `pkg/graphdb` | Done — ArangoDB driver, UpsertVertex/Edge |
| `internal/service/extract_youtube.go` | **Stub — needs implementation** |
| `internal/service/build_graph.go` | **Stub — needs implementation** |
| LLM prompts (classify + extract) | **Not yet written** |
| Tests | **None yet** |


https://www.youtube.com/watch?v=01V3IEn2MsY
https://www.youtube.com/watch?v=05kNCpqs_ZI
https://www.youtube.com/watch?v=vFb_hb_npV4
https://www.youtube.com/watch?v=-loW7xS73BM
https://www.youtube.com/watch?v=bufMa2Oscok
https://www.youtube.com/watch?v=dASaGYbN0dw
https://www.youtube.com/watch?v=j6v2uziF9Z4

https://www.youtube.com/watch?v=jIJC0RPpJuA
https://www.youtube.com/watch?v=4jBheSk3dEo
https://www.youtube.com/watch?v=3LqSJPjb1Qc