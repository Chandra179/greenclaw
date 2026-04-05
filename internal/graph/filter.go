package graph

import "strings"

// junkPredicates are predicates that carry no meaningful graph signal.
var junkPredicates = map[string]struct{}{
	"said":      {},
	"says":      {},
	"talked":    {},
	"discussed": {},
	"asked":     {},
	"answered":  {},
	"told":      {},
	"noted":     {},
	"added":     {},
	"continued": {},
	"explained": {},
	"stated":    {},
	"replied":   {},
}

// Filter removes low-signal triples using rule-based heuristics.
// No LLM call — purely deterministic.
func Filter(triples []Triple) []Triple {
	out := triples[:0]
	for _, t := range triples {
		if isJunk(t) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func isJunk(t Triple) bool {
	// Self-loop.
	if t.FromKey == t.ToKey {
		return true
	}
	// Very short keys are likely noise.
	if len(t.FromKey) < 2 || len(t.ToKey) < 2 {
		return true
	}
	// Junk predicate.
	pred := strings.ToLower(t.Predicate)
	if _, bad := junkPredicates[pred]; bad {
		return true
	}
	return false
}
