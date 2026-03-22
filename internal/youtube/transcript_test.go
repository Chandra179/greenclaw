package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pkgyt "greenclaw/pkg/youtube"

	ytlib "github.com/kkdai/youtube/v2"
)

func TestGetTranscript(t *testing.T) {
	captionXML := `<?xml version="1.0" encoding="utf-8"?>
<transcript>
  <text start="0.0" dur="2.0">First line</text>
  <text start="2.0" dur="2.0">Second line</text>
</transcript>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(captionXML))
	}))
	defer srv.Close()

	client := &Client{pkg: pkgyt.New(srv.Client())}

	video := &ytlib.Video{
		CaptionTracks: []ytlib.CaptionTrack{
			{
				BaseURL:      srv.URL + "/captions",
				LanguageCode: "en",
				Name: struct {
					SimpleText string `json:"simpleText"`
				}{SimpleText: "English"},
			},
		},
	}

	track, entries, err := client.GetTranscript(context.Background(), video, "en")
	if err != nil {
		t.Fatalf("GetTranscript() error = %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}

	if track.LanguageCode != "en" {
		t.Errorf("track.LanguageCode = %q, want %q", track.LanguageCode, "en")
	}

	want := "First line Second line"
	if track.Text != want {
		t.Errorf("track.Text = %q, want %q", track.Text, want)
	}
}

func TestGetTranscriptNotFound(t *testing.T) {
	client := &Client{pkg: pkgyt.New(http.DefaultClient)}

	video := &ytlib.Video{
		CaptionTracks: []ytlib.CaptionTrack{},
	}

	_, _, err := client.GetTranscript(context.Background(), video, "en")
	if err == nil {
		t.Fatal("expected error for missing language")
	}
}
