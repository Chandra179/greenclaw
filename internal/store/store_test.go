package store

import (
	"testing"
	"time"
)

func TestStorePutGet(t *testing.T) {
	s := New()
	r := &Result{
		URL:         "https://example.com",
		ContentType: ContentHTML,
		Title:       "Example",
		FetchedAt:   time.Now(),
	}

	s.Put(r)

	got, ok := s.Get("https://example.com")
	if !ok {
		t.Fatal("expected to find result")
	}
	if got.Title != "Example" {
		t.Errorf("got title %q, want %q", got.Title, "Example")
	}
}

func TestStoreGetMissing(t *testing.T) {
	s := New()
	_, ok := s.Get("https://missing.com")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestStoreAllAndCount(t *testing.T) {
	s := New()
	s.Put(&Result{URL: "https://a.com", FetchedAt: time.Now()})
	s.Put(&Result{URL: "https://b.com", FetchedAt: time.Now()})

	if s.Count() != 2 {
		t.Errorf("got count %d, want 2", s.Count())
	}
	if len(s.All()) != 2 {
		t.Errorf("got %d results, want 2", len(s.All()))
	}
}

func TestStoreOverwrite(t *testing.T) {
	s := New()
	s.Put(&Result{URL: "https://a.com", Title: "v1", FetchedAt: time.Now()})
	s.Put(&Result{URL: "https://a.com", Title: "v2", FetchedAt: time.Now()})

	if s.Count() != 1 {
		t.Errorf("got count %d, want 1", s.Count())
	}
	got, _ := s.Get("https://a.com")
	if got.Title != "v2" {
		t.Errorf("got title %q, want %q", got.Title, "v2")
	}
}
