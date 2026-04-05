package graph

// Weight counts duplicate triples across all chunks and returns a deduplicated
// list where each triple carries an occurrence count as its weight.
// Input triples may contain duplicates if the same relationship appeared in
// multiple chunks; those are merged here and their weight is incremented.
func Weight(triples []Triple) []WeightedTriple {
	type key struct{ from, pred, to string }
	counts := map[key]int{}
	order := []key{}

	for _, t := range triples {
		k := key{t.FromKey, t.Predicate, t.ToKey}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}

	out := make([]WeightedTriple, 0, len(order))
	for _, k := range order {
		out = append(out, WeightedTriple{
			Triple: Triple{
				FromKey:   k.from,
				Predicate: k.pred,
				ToKey:     k.to,
			},
			Weight: counts[k],
		})
	}
	return out
}
