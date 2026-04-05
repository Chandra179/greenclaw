package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"greenclaw/pkg/llm"
)

// Extract runs the LLM over each chunk and merges raw entities and triples.
// It injects a brief context note from the previous chunk into each prompt
// so the model maintains continuity across chunk boundaries.
func Extract(ctx context.Context, lc llmClient, chunks []string, category string, numCtx int) ([]RawEntity, []RawTriple, error) {
	ontology := ontologyFor(category)

	var (
		seenEntities = map[string]RawEntity{} // name → entity (case-insensitive key)
		seenTriples  = map[string]struct{}{}
		allTriples   []RawTriple
		prevContext  string // brief summary injected into next chunk's prompt
	)

	for i, chunk := range chunks {
		prompt := buildExtractionPrompt(i+1, len(chunks), chunk, ontology, prevContext)

		resp, err := lc.Chat(ctx, llm.ChatRequest{
			Prompt: prompt,
			Schema: extractionSchema,
			NumCtx: numCtx,
		})
		if err != nil {
			log.Printf("[graph/extract] warn: LLM failed on chunk %d/%d: %v", i+1, len(chunks), err)
			continue
		}

		var out extractionOutput
		if err := json.Unmarshal(resp.JsonResponse, &out); err != nil {
			log.Printf("[graph/extract] warn: parse failed on chunk %d/%d: %v", i+1, len(chunks), err)
			continue
		}

		// Collect entities; first occurrence wins for type.
		entityNamesInChunk := make([]string, 0, len(out.Entities))
		for _, e := range out.Entities {
			name := strings.TrimSpace(e.Name)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if _, exists := seenEntities[key]; !exists {
				seenEntities[key] = RawEntity{Name: name, Type: e.Type}
			}
			entityNamesInChunk = append(entityNamesInChunk, name)
		}

		// Collect triples; deduplicate by (subject, predicate, object).
		for _, r := range out.Relationships {
			subj := strings.TrimSpace(r.Subject)
			pred := strings.TrimSpace(r.Predicate)
			obj := strings.TrimSpace(r.Object)
			if subj == "" || pred == "" || obj == "" {
				continue
			}
			key := strings.ToLower(subj) + "|" + strings.ToLower(pred) + "|" + strings.ToLower(obj)
			if _, exists := seenTriples[key]; !exists {
				seenTriples[key] = struct{}{}
				allTriples = append(allTriples, RawTriple{
					Subject:   subj,
					Predicate: pred,
					Object:    obj,
				})
			}
		}

		// Build a brief context note for the next chunk: list top entities found.
		prevContext = buildContextNote(entityNamesInChunk)
	}

	entities := make([]RawEntity, 0, len(seenEntities))
	for _, e := range seenEntities {
		entities = append(entities, e)
	}
	return entities, allTriples, nil
}

func buildExtractionPrompt(chunkNum, total int, chunk string, ontology []string, prevContext string) string {
	var sb strings.Builder

	sb.WriteString("Extract entities and relationships from this transcript chunk.\n\n")
	fmt.Fprintf(&sb, "Entity types (use only these): %s\n\n", strings.Join(ontology, ", "))
	sb.WriteString("For relationships, use descriptive predicate verbs (e.g. 'developed', 'competes_with', 'introduced').\n\n")

	if prevContext != "" {
		fmt.Fprintf(&sb, "Context from previous segment: %s\n\n", prevContext)
	}

	fmt.Fprintf(&sb, "Chunk %d of %d:\n%s\n\nRespond with JSON only.", chunkNum, total, chunk)
	return sb.String()
}

// buildContextNote summarises a list of entity names into a short string
// injected as context into the next chunk's prompt. No extra LLM call needed.
func buildContextNote(names []string) string {
	if len(names) == 0 {
		return ""
	}
	limit := min(10, len(names))
	return "Key entities mentioned: " + strings.Join(names[:limit], ", ") + "."
}
