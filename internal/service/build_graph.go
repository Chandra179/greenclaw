package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"greenclaw/pkg/chunker"
	"greenclaw/pkg/graphdb"
	"greenclaw/pkg/llm"
)

type BuildGraphReq struct {
	YoutubeURL string
}

type BuildGraphResp struct {
	VideoID       string `json:"video_id"`
	Category      string `json:"category"`
	EntitiesAdded int    `json:"entities_added"`
	EdgesAdded    int    `json:"edges_added"`
}

// videoDoc is the shape stored in the videos collection.
type videoDoc struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Transcript string `json:"transcript"`
	Language   string `json:"language"`
	Duration   string `json:"duration"`
	Category   string `json:"category"`
	Processed  bool   `json:"processed"`
}

// classifyResponse is the LLM output for category classification.
type classifyResponse struct {
	Category string `json:"category"`
}

// extractResponse is the LLM output for entity/relationship extraction.
type extractResponse struct {
	Entities      []entityItem       `json:"entities"`
	Relationships []relationshipItem `json:"relationships"`
}

type entityItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type relationshipItem struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

var classifySchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "category": { "type": "string", "enum": ["economy", "technology"] }
  },
  "required": ["category"]
}`)

var extractSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "entities": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "type": { "type": "string", "enum": ["concept", "person", "organization", "event"] }
        },
        "required": ["name", "type"]
      }
    },
    "relationships": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "from": { "type": "string" },
          "to": { "type": "string" },
          "type": { "type": "string" }
        },
        "required": ["from", "to", "type"]
      }
    }
  },
  "required": ["entities", "relationships"]
}`)

func (d *Dependencies) BuildGraph(ctx context.Context, req BuildGraphReq) (*BuildGraphResp, error) {
	if d.GraphDB == nil {
		return nil, fmt.Errorf("graph DB not configured")
	}
	if d.LLMClient == nil {
		return nil, fmt.Errorf("LLM client not configured")
	}

	videoID, err := extractVideoID(req.YoutubeURL)
	if err != nil {
		return nil, fmt.Errorf("parse video ID: %w", err)
	}

	// Load video from graph DB.
	var vDoc videoDoc
	if err := d.GraphDB.GetNode(ctx, "Video", videoID, &vDoc); err != nil {
		return nil, fmt.Errorf("video %s not found in graph DB (run /extract/youtube first): %w", videoID, err)
	}
	if vDoc.Transcript == "" {
		return nil, fmt.Errorf("video %s has no transcript", videoID)
	}

	// 2a. Classify category.
	category := vDoc.Category
	if category == "" {
		category, err = d.classifyVideo(ctx, vDoc.Transcript)
		if err != nil {
			return nil, fmt.Errorf("classify video: %w", err)
		}
		// Persist category back.
		if err := d.GraphDB.UpsertNode(ctx, "Video", videoID, map[string]interface{}{
			"category": category,
		}); err != nil {
			log.Printf("[graph] warn: failed to update category for %s: %v", videoID, err)
		}
		log.Printf("[graph] classified %s as %q", videoID, category)
	}

	// 2b+2c. Extract entities and relationships (chunked).
	allEntities, allRelationships, err := d.extractFromTranscript(ctx, vDoc.Transcript)
	if err != nil {
		return nil, fmt.Errorf("extract entities: %w", err)
	}
	log.Printf("[graph] extracted %d entities, %d relationships from %s",
		len(allEntities), len(allRelationships), videoID)

	// Deduplicate by slug.
	entityDocs, slugMap := buildEntityDocs(allEntities, category)

	// Write entity nodes.
	if err := d.GraphDB.BulkUpsertNodes(ctx, "Entity", entityDocs); err != nil {
		return nil, fmt.Errorf("upsert entities: %w", err)
	}

	// Write RELATED_TO relationships: Entity → Entity
	rels := buildRelatedEdges(allRelationships, slugMap)
	if len(rels) > 0 {
		if err := d.GraphDB.BulkUpsertRelationships(ctx, rels); err != nil {
			return nil, fmt.Errorf("upsert related_to edges: %w", err)
		}
	}

	// Mark video as processed.
	if err := d.GraphDB.UpsertNode(ctx, "Video", videoID, map[string]interface{}{
		"processed": true,
	}); err != nil {
		log.Printf("[graph] warn: failed to mark %s as processed: %v", videoID, err)
	}

	log.Printf("[graph] done for %s: %d entities, %d related_to",
		videoID, len(entityDocs), len(rels))

	return &BuildGraphResp{
		VideoID:       videoID,
		Category:      category,
		EntitiesAdded: len(entityDocs),
		EdgesAdded:    len(rels),
	}, nil
}

// classifyVideo asks the LLM to assign a category to the transcript.
func (d *Dependencies) classifyVideo(ctx context.Context, transcript string) (string, error) {
	// Use only the first ~2000 chars for classification — enough context, cheap call.
	excerpt := transcript
	if len(excerpt) > 2000 {
		excerpt = excerpt[:2000]
	}

	prompt := fmt.Sprintf(`Classify the following video transcript into exactly one category.
Categories:
- economy: macroeconomics, finance, markets, trade
- technology: software, hardware, AI, engineering

Transcript excerpt:
%s

Respond with JSON only.`, excerpt)

	resp, err := d.LLMClient.Chat(ctx, llm.ChatRequest{
		Prompt: prompt,
		Schema: classifySchema,
		NumCtx: d.Cfg.LLM.NumCtx,
	})
	if err != nil {
		return "", fmt.Errorf("LLM classify: %w", err)
	}

	var cr classifyResponse
	if err := json.Unmarshal(resp.JsonResponse, &cr); err != nil {
		return "", fmt.Errorf("parse classify response: %w", err)
	}
	switch cr.Category {
	case "economy", "technology":
		return cr.Category, nil
	default:
		return "technology", nil // safe default
	}
}

// extractFromTranscript chunks the transcript and extracts entities and
// relationships from each chunk, then merges and deduplicates the results.
func (d *Dependencies) extractFromTranscript(ctx context.Context, transcript string) ([]entityItem, []relationshipItem, error) {
	const charsPerToken = 4
	const reservedTokens = 500
	chunkSize := (d.Cfg.LLM.NumCtx - reservedTokens) * charsPerToken
	if chunkSize <= 0 {
		chunkSize = 8000
	}
	overlap := d.Cfg.LLM.OverlapTokens * charsPerToken

	rc := chunker.RecursiveChunker{ChunkSize: chunkSize, Overlap: overlap}
	chunks := rc.Chunk(transcript)

	seenEntities := map[string]entityItem{}
	seenRels := map[string]struct{}{}
	var allRels []relationshipItem

	for i, chunk := range chunks {
		prompt := fmt.Sprintf(`Extract entities and relationships from this transcript chunk.

Entity types: concept, person, organization, event
Relationship type: related_to

Chunk %d of %d:
%s

Respond with JSON only.`, i+1, len(chunks), chunk)

		resp, err := d.LLMClient.Chat(ctx, llm.ChatRequest{
			Prompt: prompt,
			Schema: extractSchema,
			NumCtx: d.Cfg.LLM.NumCtx,
		})
		if err != nil {
			log.Printf("[graph] warn: LLM extract failed on chunk %d: %v", i+1, err)
			continue
		}

		var er extractResponse
		if err := json.Unmarshal(resp.JsonResponse, &er); err != nil {
			log.Printf("[graph] warn: parse extract response chunk %d: %v", i+1, err)
			continue
		}

		for _, e := range er.Entities {
			slug := toSlug(e.Name)
			if slug == "" {
				continue
			}
			if _, exists := seenEntities[slug]; !exists {
				seenEntities[slug] = entityItem{Name: e.Name, Type: e.Type}
			}
		}

		for _, r := range er.Relationships {
			fromSlug := toSlug(r.From)
			toSlug_ := toSlug(r.To)
			if fromSlug == "" || toSlug_ == "" {
				continue
			}
			key := fromSlug + "|" + toSlug_ + "|" + r.Type
			if _, exists := seenRels[key]; !exists {
				seenRels[key] = struct{}{}
				allRels = append(allRels, relationshipItem{
					From: r.From,
					To:   r.To,
					Type: r.Type,
				})
			}
		}
	}

	entities := make([]entityItem, 0, len(seenEntities))
	for _, e := range seenEntities {
		entities = append(entities, e)
	}
	return entities, allRels, nil
}

// buildEntityDocs converts entity items to graph DB vertex documents.
// Returns the docs and a slug→name map for edge building.
func buildEntityDocs(entities []entityItem, category string) ([]map[string]interface{}, map[string]string) {
	docs := make([]map[string]interface{}, 0, len(entities))
	slugMap := make(map[string]string, len(entities))
	for _, e := range entities {
		slug := toSlug(e.Name)
		if slug == "" {
			continue
		}
		slugMap[slug] = e.Name
		docs = append(docs, map[string]interface{}{
			"key":      slug,
			"name":     e.Name,
			"type":     e.Type,
			"category": category,
		})
	}
	return docs, slugMap
}

// buildRelatedEdges converts relationship items to Relationship list, only for
// entities whose slugs exist in the known slug map.
func buildRelatedEdges(rels []relationshipItem, slugMap map[string]string) []graphdb.Relationship {
	edges := make([]graphdb.Relationship, 0, len(rels))
	for _, r := range rels {
		fromSlug := toSlug(r.From)
		toSlug_ := toSlug(r.To)
		if _, ok := slugMap[fromSlug]; !ok {
			continue
		}
		if _, ok := slugMap[toSlug_]; !ok {
			continue
		}
		edges = append(edges, graphdb.Relationship{
			FromLabel: "Entity",
			FromKey:   fromSlug,
			Type:      r.Type,
			ToLabel:   "Entity",
			ToKey:     toSlug_,
		})
	}
	return edges
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// toSlug normalises an entity name to a URL-safe key.
func toSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphanumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
