package router

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"strings"
)

// ContentType represents the detected content type of a URL.
type ContentType string

const (
	ContentHTML   ContentType = "html"
	ContentJSON   ContentType = "json"
	ContentXML    ContentType = "xml"
	ContentBinary ContentType = "binary"
)

// HTTPDoer abstracts HTTP request execution for testability.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Classify performs a HEAD request and returns the content type classification.
func Classify(ctx context.Context, client HTTPDoer, url string) (ContentType, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating HEAD request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HEAD request: %w", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		// Fall back to HTML if no content type header
		return ContentHTML, nil
	}

	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		// If we can't parse, try raw string matching
		mediaType = strings.ToLower(strings.TrimSpace(ct))
	}

	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		return ContentHTML, nil
	case mediaType == "application/json":
		return ContentJSON, nil
	case mediaType == "text/xml" || mediaType == "application/xml" ||
		strings.HasSuffix(mediaType, "+xml"):
		return ContentXML, nil
	case strings.HasPrefix(mediaType, "image/") ||
		mediaType == "application/pdf" ||
		mediaType == "application/octet-stream":
		return ContentBinary, nil
	default:
		// Default to HTML for unknown text types, binary for others
		if strings.HasPrefix(mediaType, "text/") {
			return ContentHTML, nil
		}
		return ContentBinary, nil
	}
}
