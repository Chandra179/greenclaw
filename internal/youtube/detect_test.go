package youtube

import (
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantType URLType
		wantID   string
		wantOK   bool
	}{
		// Video URLs
		{
			name:     "standard watch URL",
			url:      "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			wantType: VideoURL,
			wantID:   "dQw4w9WgXcQ",
			wantOK:   true,
		},
		{
			name:     "watch URL with extra params",
			url:      "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s&list=PLtest",
			wantType: VideoURL,
			wantID:   "dQw4w9WgXcQ",
			wantOK:   true,
		},
		{
			name:     "short URL",
			url:      "https://youtu.be/dQw4w9WgXcQ",
			wantType: VideoURL,
			wantID:   "dQw4w9WgXcQ",
			wantOK:   true,
		},
		{
			name:     "shorts URL",
			url:      "https://www.youtube.com/shorts/dQw4w9WgXcQ",
			wantType: VideoURL,
			wantID:   "dQw4w9WgXcQ",
			wantOK:   true,
		},
		{
			name:     "embed URL",
			url:      "https://www.youtube.com/embed/dQw4w9WgXcQ",
			wantType: VideoURL,
			wantID:   "dQw4w9WgXcQ",
			wantOK:   true,
		},
		{
			name:     "mobile URL",
			url:      "https://m.youtube.com/watch?v=dQw4w9WgXcQ",
			wantType: VideoURL,
			wantID:   "dQw4w9WgXcQ",
			wantOK:   true,
		},

		// Playlist URLs
		{
			name:     "playlist URL",
			url:      "https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf",
			wantType: PlaylistURL,
			wantID:   "PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf",
			wantOK:   true,
		},

		// Channel URLs
		{
			name:     "channel URL",
			url:      "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw",
			wantType: ChannelURL,
			wantID:   "UCuAXFkgsw1L7xaCfnd5JJOw",
			wantOK:   true,
		},
		{
			name:     "handle URL",
			url:      "https://www.youtube.com/@MrBeast",
			wantType: ChannelURL,
			wantID:   "@MrBeast",
			wantOK:   true,
		},
		{
			name:     "handle URL with trailing path",
			url:      "https://www.youtube.com/@MrBeast/videos",
			wantType: ChannelURL,
			wantID:   "@MrBeast",
			wantOK:   true,
		},

		// Non-YouTube URLs
		{
			name:   "non-YouTube URL",
			url:    "https://www.google.com/search?q=test",
			wantOK: false,
		},
		{
			name:   "empty string",
			url:    "",
			wantOK: false,
		},
		{
			name:   "YouTube homepage",
			url:    "https://www.youtube.com/",
			wantOK: false,
		},
		{
			name:   "watch URL without video ID",
			url:    "https://www.youtube.com/watch",
			wantOK: false,
		},
		{
			name:   "invalid URL",
			url:    "not a url at all",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotID, gotOK := Detect(tt.url)
			if gotOK != tt.wantOK {
				t.Errorf("Detect(%q) ok = %v, want %v", tt.url, gotOK, tt.wantOK)
				return
			}
			if !tt.wantOK {
				return
			}
			if gotType != tt.wantType {
				t.Errorf("Detect(%q) type = %v, want %v", tt.url, gotType, tt.wantType)
			}
			if gotID != tt.wantID {
				t.Errorf("Detect(%q) id = %q, want %q", tt.url, gotID, tt.wantID)
			}
		})
	}
}
