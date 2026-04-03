package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        ContentType
	}{
		{"html", "text/html; charset=utf-8", ContentHTML},
		{"xhtml", "application/xhtml+xml", ContentHTML},
		{"json", "application/json", ContentJSON},
		{"xml", "text/xml", ContentXML},
		{"app-xml", "application/xml", ContentXML},
		{"atom", "application/atom+xml", ContentXML},
		{"pdf", "application/pdf", ContentBinary},
		{"image-png", "image/png", ContentBinary},
		{"image-jpeg", "image/jpeg", ContentBinary},
		{"octet-stream", "application/octet-stream", ContentBinary},
		{"empty", "", ContentHTML},
		{"plain-text", "text/plain", ContentHTML},
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
