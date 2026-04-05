# Build Graph Pipeline

Converts a YouTube video transcript into a structured knowledge graph of entities and their relationships, stored in Neo4j.

---

## Pipeline Overview

```
Transcript
  → Classify
  → Extract
  → Resolve
  → Filter
  → Weight
  → Store
```

Each stage takes the output of the previous. Skipping a stage means removing a function call in the orchestrator.

---

## Stages

### 1. Classify
**Goal:** Assign the video to a domain category (e.g. technology, economy).

Only the first portion of the transcript is sent to the LLM. The category is cached on the Video node so it is only computed once.

**Why it matters:** Later stages use the category to select a domain-specific entity ontology. A technology video extracts Models, Frameworks, and Researchers — an economy video extracts Policies, Institutions, and Metrics. Without classification, extraction uses a generic fallback type list that produces noisier results.

**Tradeoff:** Using only the opening excerpt is cheap but can misclassify videos that take time to get to their main topic.

---

### 2. Extract
**Goal:** Pull raw entities and subject-predicate-object triples from the transcript, one chunk at a time.

The transcript is split into overlapping chunks. Each chunk is sent to the LLM with the domain ontology injected into the prompt, so the model only produces entity types that belong to the video's category. A brief note about the key entities found in the previous chunk is prepended to each prompt to maintain continuity across boundaries.

**Why it matters:** LLMs have finite context windows, so chunking is unavoidable for long transcripts. Overlap and context injection reduce the chance of an entity mentioned across a chunk boundary being missed or misidentified.

**Tradeoff:** More chunks mean more LLM calls and more cost. Overlapping chunks introduce some duplicate extractions, which later stages must clean up. Context injection is cheap (no extra call) but is a summarised hint, not the full prior chunk — some cross-chunk relationships may still be missed.

---

### 3. Resolve
**Goal:** Deduplicate entities that refer to the same real-world thing, and normalise relationship predicates to a consistent format.

All extracted entity names are batched and sent to the LLM with a prompt asking it to group synonyms and aliases under a single canonical name (e.g. "LLM", "Large Language Model", "The Model" → "LLM"). A mapping table is built from the response and applied to every triple so all edges point to the same canonical node. Predicates are normalised to lowercase snake_case.

**Why it matters:** Without resolution, the graph becomes fragmented. Three nodes for the same concept mean three disconnected subgraphs instead of one well-connected hub. This is the most impactful quality step in the pipeline.

**Tradeoff:** Each batch requires an LLM call. On transcripts with hundreds of unique entities, this adds latency and cost. The LLM can also make wrong groupings — merging entities that should remain distinct. Embedding-based clustering would be more deterministic but requires a separate embedding model.

---

### 4. Filter
**Goal:** Remove triples that carry no meaningful graph signal.

Rule-based only — no LLM call. Removes self-loops, triples with very short node keys, and triples whose predicate is a generic speech verb (said, mentioned, explained, etc.).

**Why it matters:** Raw extraction always produces noise. Keeping junk triples pollutes the graph and degrades retrieval quality downstream.

**Tradeoff:** Rule-based filtering is fast and predictable but has a fixed vocabulary of bad predicates. An LLM-based filtering pass would catch more subtle junk but at extra cost. The current approach is conservative — it only removes clearly useless triples.

---

### 5. Weight
**Goal:** Count how many times the same relationship appeared across different chunks and assign that count as the edge weight.

No LLM call. Duplicate triples (same subject, predicate, object) are merged into one weighted triple.

**Why it matters:** A relationship mentioned ten times throughout a video is more significant than one mentioned once. Weight makes this signal explicit in the graph, enabling downstream consumers to rank or filter by relationship strength.

**Tradeoff:** Weight only reflects extraction frequency, not semantic importance. A repeated filler phrase can accumulate weight the same as a genuinely central relationship.

---

### 6. Store
**Goal:** Write the resolved entities as nodes and the weighted triples as relationships into Neo4j.

Edges whose endpoints are not in the known entity set are silently dropped. On re-runs, edge weights are accumulated (not replaced), so repeated processing of the same video increases weights rather than resetting them.

**Why it matters:** Keeping upserts idempotent means the pipeline is safe to re-run after partial failures without duplicating data.

**Tradeoff:** Accumulating weights on re-runs means running the pipeline twice on the same video doubles all edge weights. The Video node is marked `processed: true` to prevent accidental re-runs, but this flag must be manually cleared to reprocess.
