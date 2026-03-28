package store

import (
	"sync"
	"time"
)

type ContentType string

const (
	ContentHTML   ContentType = "html"
	ContentJSON   ContentType = "json"
	ContentXML    ContentType = "xml"
	ContentBinary ContentType = "binary"
)

type Result struct {
	URL         string      `json:"url"`
	ContentType ContentType `json:"content_type"`
	Title       string      `json:"title,omitempty"`
	Description string      `json:"description,omitempty"`
	Text        string      `json:"-"`
	Links       []string    `json:"links,omitempty"`
	FilePath    string      `json:"-"` // for binary downloads
	YouTube     *YouTubeData `json:"youtube,omitempty"`
	Error       string       `json:"error,omitempty"`
	FetchedAt   time.Time   `json:"fetched_at"`
	Model       string      `json:"model,omitempty"`
	NumCtx      int         `json:"num_ctx,omitempty"`
	Styles      []string    `json:"styles,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	results map[string]*Result
}

func New() *Store {
	return &Store{
		results: make(map[string]*Result),
	}
}

func (s *Store) Put(r *Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[r.URL] = r
}

func (s *Store) Get(url string) (*Result, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.results[url]
	return r, ok
}

func (s *Store) All() []*Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]*Result, 0, len(s.results))
	for _, r := range s.results {
		results = append(results, r)
	}
	return results
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.results)
}
