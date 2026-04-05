package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"greenclaw/pkg/llm"
)

const resolveBatchSize = 50

// Resolve deduplicates entities using an LLM batch-prompt pass, builds a
// canonical name mapping, then applies it to all raw triples.
// Returns the canonical entity list and the resolved triples.
func Resolve(ctx context.Context, lc llmClient, rawEntities []RawEntity, rawTriples []RawTriple, category string, numCtx int) ([]Entity, []Triple, error) {
	// Build canonical mapping: lowercase(name) → canonical name.
	mapping, entityTypes := buildCanonicalMapping(ctx, lc, rawEntities, numCtx)

	// Build Entity list from the canonical names.
	seenCanonical := map[string]struct{}{}
	var entities []Entity
	for _, e := range rawEntities {
		canonical := resolveEntity(e.Name, mapping)
		key := toSlug(canonical)
		if key == "" {
			continue
		}
		if _, exists := seenCanonical[key]; exists {
			continue
		}
		seenCanonical[key] = struct{}{}

		// Prefer the type of the canonical name if available, else use original.
		etype := e.Type
		if t, ok := entityTypes[strings.ToLower(canonical)]; ok {
			etype = t
		}

		entities = append(entities, Entity{
			Key:      key,
			Name:     canonical,
			Type:     etype,
			Category: category,
		})
	}

	// Apply mapping to triples and resolve predicate to snake_case.
	seenTriple := map[string]struct{}{}
	var triples []Triple
	for _, r := range rawTriples {
		fromName := resolveEntity(r.Subject, mapping)
		toName := resolveEntity(r.Object, mapping)
		pred := canonicalizePredicate(r.Predicate)

		fromKey := toSlug(fromName)
		toKey := toSlug(toName)
		if fromKey == "" || toKey == "" || fromKey == toKey {
			continue
		}

		key := fromKey + "|" + pred + "|" + toKey
		if _, exists := seenTriple[key]; exists {
			continue
		}
		seenTriple[key] = struct{}{}
		triples = append(triples, Triple{
			FromKey:   fromKey,
			Predicate: pred,
			ToKey:     toKey,
		})
	}

	return entities, triples, nil
}

// buildCanonicalMapping sends entity names in batches to the LLM and collects
// alias → canonical mappings. Also returns a map of lowercase name → type.
func buildCanonicalMapping(ctx context.Context, lc llmClient, entities []RawEntity, numCtx int) (map[string]string, map[string]string) {
	mapping := map[string]string{}   // lowercase(alias) → canonical name
	entityTypes := map[string]string{} // lowercase(name) → type

	for _, e := range entities {
		entityTypes[strings.ToLower(e.Name)] = e.Type
	}

	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = e.Name
	}

	for i := 0; i < len(names); i += resolveBatchSize {
		end := min(i+resolveBatchSize, len(names))
		batch := names[i:end]

		groups, err := resolveEntityBatch(ctx, lc, batch, numCtx)
		if err != nil {
			log.Printf("[graph/resolve] warn: resolution batch %d failed: %v", i/resolveBatchSize+1, err)
			// On failure, treat each name as its own canonical.
			for _, n := range batch {
				mapping[strings.ToLower(n)] = n
			}
			continue
		}

		for _, g := range groups {
			canonical := strings.TrimSpace(g.Canonical)
			if canonical == "" {
				continue
			}
			mapping[strings.ToLower(canonical)] = canonical
			for _, alias := range g.Aliases {
				alias = strings.TrimSpace(alias)
				if alias != "" {
					mapping[strings.ToLower(alias)] = canonical
				}
			}
		}
	}

	// Ensure every entity name has a mapping entry (self-map if not grouped).
	for _, n := range names {
		if _, ok := mapping[strings.ToLower(n)]; !ok {
			mapping[strings.ToLower(n)] = n
		}
	}

	return mapping, entityTypes
}

type resolveGroup struct {
	Canonical string
	Aliases   []string
}

func resolveEntityBatch(ctx context.Context, lc llmClient, names []string, numCtx int) ([]resolveGroup, error) {
	listed := make([]string, len(names))
	for i, n := range names {
		listed[i] = fmt.Sprintf("- %s", n)
	}

	prompt := fmt.Sprintf(`The following is a list of entity names extracted from a transcript.
Group together any names that refer to the same real-world entity.
For each group, choose the most canonical/common name as "canonical" and list the others as "aliases".
Only include names that actually appear in the list below. Do not invent new names.

Entities:
%s

Respond with JSON only.`, strings.Join(listed, "\n"))

	resp, err := lc.Chat(ctx, llm.ChatRequest{
		Prompt: prompt,
		Schema: resolutionSchema,
		NumCtx: numCtx,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM resolve: %w", err)
	}

	var out resolutionOutput
	if err := json.Unmarshal(resp.JsonResponse, &out); err != nil {
		return nil, fmt.Errorf("parse resolve response: %w", err)
	}

	groups := make([]resolveGroup, len(out.Groups))
	for i, g := range out.Groups {
		groups[i] = resolveGroup{Canonical: g.Canonical, Aliases: g.Aliases}
	}
	return groups, nil
}

// resolveEntity looks up a name in the canonical mapping (case-insensitive).
// Falls back to the original name if not found.
func resolveEntity(name string, mapping map[string]string) string {
	if canonical, ok := mapping[strings.ToLower(strings.TrimSpace(name))]; ok {
		return canonical
	}
	return name
}

var (
	nonAlphanumRe  = regexp.MustCompile(`[^a-z0-9]+`)
	whitespaceRe   = regexp.MustCompile(`\s+`)
)

// toSlug converts a name to a lowercase hyphen-separated key.
func toSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphanumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// canonicalizePredicate converts a predicate to lowercase_snake_case.
func canonicalizePredicate(pred string) string {
	s := strings.ToLower(strings.TrimSpace(pred))
	s = whitespaceRe.ReplaceAllString(s, "_")
	s = nonAlphanumRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "related_to"
	}
	return s
}
