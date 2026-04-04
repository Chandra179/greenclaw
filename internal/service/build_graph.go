package service

type BuildGraphReq struct {
	YoutubeURL string
}

func (d *Dependencies) BuildGraph(req BuildGraphReq) {
	// given youtube url check if the data exists in graphdb database,
	// that previously inserted in @internal/orchestrator/extract_youtube.go
	// the graph builder is grouped per category like economy, finance, technology
	// then we need entity normalization
}
