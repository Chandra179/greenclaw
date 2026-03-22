package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const testHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
    <meta name="description" content="A test page for scraping">
</head>
<body>
    <h1>Hello World</h1>
    <p>This is a test paragraph with enough content.</p>
    <a href="https://example.com/page1">Page 1</a>
    <a href="https://example.com/page2">Page 2</a>
    <a href="#section">Anchor</a>
    <a href="javascript:void(0)">JS Link</a>
    <script>console.log("hi")</script>
    <style>body { color: red; }</style>
</body>
</html>`

func TestFetchHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(testHTML))
	}))
	defer srv.Close()

	result, err := FetchHTML(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Test Page" {
		t.Errorf("got title %q, want %q", result.Title, "Test Page")
	}
	if result.Description != "A test page for scraping" {
		t.Errorf("got description %q, want %q", result.Description, "A test page for scraping")
	}
	// Should have 2 valid links (anchors and js: links filtered out)
	if len(result.Links) != 2 {
		t.Errorf("got %d links, want 2: %v", len(result.Links), result.Links)
	}
	// Text should not contain script/style content
	if result.Text == "" {
		t.Error("text should not be empty")
	}
}

func TestFetchHTMLEscalation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte("<html><body>Blocked</body></html>"))
	}))
	defer srv.Close()

	_, err := FetchHTML(context.Background(), srv.Client(), srv.URL)
	if err != ErrNeedsEscalation {
		t.Errorf("got error %v, want ErrNeedsEscalation", err)
	}
}

func TestDownloadBinary(t *testing.T) {
	content := []byte("fake PDF content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	result, err := DownloadBinary(context.Background(), srv.Client(), srv.URL+"/test.pdf", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FilePath != filepath.Join(dir, "test.pdf") {
		t.Errorf("got filepath %q, want %q", result.FilePath, filepath.Join(dir, "test.pdf"))
	}

	data, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("got content %q, want %q", data, content)
	}
}

func TestFetchJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key": "value"}`))
	}))
	defer srv.Close()

	result, err := FetchJSON(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != "json" {
		t.Errorf("got type %q, want json", result.ContentType)
	}
}
