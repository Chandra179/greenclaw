package pipeline

import (
	"sync"

	"greenclaw/internal/result"
)

// Result is an alias for result.Result for convenience within the pipeline package.
type Result = result.Result

// ResultStore abstracts result storage for testability.
type ResultStore interface {
	Put(r *Result)
	Get(url string) (*Result, bool)
	All() []*Result
	Count() int
}

type memStore struct {
	mu      sync.RWMutex
	results map[string]*Result
}

func newStore() *memStore {
	return &memStore{
		results: make(map[string]*Result),
	}
}

func (s *memStore) Put(r *Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[r.URL] = r
}

func (s *memStore) Get(url string) (*Result, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.results[url]
	return r, ok
}

func (s *memStore) All() []*Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]*Result, 0, len(s.results))
	for _, r := range s.results {
		results = append(results, r)
	}
	return results
}

func (s *memStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.results)
}
