package service

import (
	"context"
	"fmt"
	"log"

	"greenclaw/internal/graph"
	"greenclaw/pkg/chunker"
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

// videoDoc is the shape stored in the Video node.
type videoDoc struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Transcript string `json:"transcript"`
	Language   string `json:"language"`
	Duration   string `json:"duration"`
	Category   string `json:"category"`
	Processed  bool   `json:"processed"`
}

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

	var vDoc videoDoc
	if err := d.GraphDB.GetNode(ctx, "Video", videoID, &vDoc); err != nil {
		return nil, fmt.Errorf("video %s not found (run /extract/youtube first): %w", videoID, err)
	}
	if vDoc.Transcript == "" {
		return nil, fmt.Errorf("video %s has no transcript", videoID)
	}

	// Stage 1: classify.
	category := vDoc.Category
	if category == "" {
		category, err = graph.Classify(ctx, d.LLMClient, vDoc.Transcript, d.Cfg.LLM.NumCtx)
		if err != nil {
			return nil, fmt.Errorf("classify: %w", err)
		}
		if err := d.GraphDB.UpsertNode(ctx, "Video", videoID, map[string]interface{}{"category": category}); err != nil {
			log.Printf("[graph] warn: update category for %s: %v", videoID, err)
		}
		log.Printf("[graph] classified %s as %q", videoID, category)
	}

	// Stage 2: chunk + extract.
	const charsPerToken = 4
	chunkSize := (d.Cfg.LLM.NumCtx - 500) * charsPerToken
	if chunkSize <= 0 {
		chunkSize = 8000
	}
	rc := chunker.RecursiveChunker{ChunkSize: chunkSize, Overlap: d.Cfg.LLM.OverlapTokens * charsPerToken}
	chunks := rc.Chunk(vDoc.Transcript)

	rawEntities, rawTriples, err := graph.Extract(ctx, d.LLMClient, chunks, category, d.Cfg.LLM.NumCtx)
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	log.Printf("[graph] extracted %d raw entities, %d raw triples from %s", len(rawEntities), len(rawTriples), videoID)

	// Stage 3: resolve (entity dedup + predicate canonicalization).
	entities, triples, err := graph.Resolve(ctx, d.LLMClient, rawEntities, rawTriples, category, d.Cfg.LLM.NumCtx)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	log.Printf("[graph] resolved to %d entities, %d triples", len(entities), len(triples))

	// Stage 4: filter junk triples.
	triples = graph.Filter(triples)

	// Stage 5: weight (count edge frequency across chunks).
	weighted := graph.Weight(triples)

	// Stage 6: write to graph DB.
	entitiesAdded, edgesAdded, err := graph.Store(ctx, d.GraphDB, videoID, entities, weighted)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}

	// Mark video as processed.
	if err := d.GraphDB.UpsertNode(ctx, "Video", videoID, map[string]interface{}{"processed": true}); err != nil {
		log.Printf("[graph] warn: mark %s processed: %v", videoID, err)
	}

	log.Printf("[graph] done %s: %d entities, %d edges", videoID, entitiesAdded, edgesAdded)
	return &BuildGraphResp{
		VideoID:       videoID,
		Category:      category,
		EntitiesAdded: entitiesAdded,
		EdgesAdded:    edgesAdded,
	}, nil
}

