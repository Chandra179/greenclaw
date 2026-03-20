package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"greenclaw/internal/store"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        store.ContentType
	}{
		{"html", "text/html; charset=utf-8", store.ContentHTML},
		{"xhtml", "application/xhtml+xml", store.ContentHTML},
		{"json", "application/json", store.ContentJSON},
		{"xml", "text/xml", store.ContentXML},
		{"app-xml", "application/xml", store.ContentXML},
		{"atom", "application/atom+xml", store.ContentXML},
		{"pdf", "application/pdf", store.ContentBinary},
		{"image-png", "image/png", store.ContentBinary},
		{"image-jpeg", "image/jpeg", store.ContentBinary},
		{"octet-stream", "application/octet-stream", store.ContentBinary},
		{"empty", "", store.ContentHTML},
		{"plain-text", "text/plain", store.ContentHTML},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodHead {
					t.Errorf("expected HEAD, got %s", r.Method)
				}
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
			}))
			defer srv.Close()

			got, err := Classify(context.Background(), srv.Client(), srv.URL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
