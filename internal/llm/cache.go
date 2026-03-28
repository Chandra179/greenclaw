package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ResultCache stores LLM Results as JSON files on disk.
// Each file is named by a sha256 hash of (cacheKey|style|model|numCtx),
// so changing any of those parameters automatically bypasses stale entries.
type ResultCache struct {
	dir string
}

// NewResultCache creates a ResultCache rooted at dir, creating the directory if needed.
func NewResultCache(dir string) (*ResultCache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	return &ResultCache{dir: dir}, nil
}

// Get looks up a cached Result. Returns (nil, false) on any miss or read error.
func (c *ResultCache) Get(cacheKey, style, model string, numCtx int) (*Result, bool) {
	path := filepath.Join(c.dir, c.filename(cacheKey, style, model, numCtx))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, false
	}
	return &r, true
}

// Put writes a Result to the cache. Errors are non-fatal (logged by the caller).
func (c *ResultCache) Put(cacheKey, style, model string, numCtx int, r *Result) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	path := filepath.Join(c.dir, c.filename(cacheKey, style, model, numCtx))
	return os.WriteFile(path, data, 0o644)
}

func (c *ResultCache) filename(cacheKey, style, model string, numCtx int) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%d", cacheKey, style, model, numCtx)
	return hex.EncodeToString(h.Sum(nil)) + ".json"
}
