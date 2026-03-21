package youtube

import (
	"strings"
	"testing"

	ytlib "github.com/kkdai/youtube/v2"
)

func TestAudioStreamSelection(t *testing.T) {
	// Test that audio format selection logic picks opus/webm over m4a
	formats := ytlib.FormatList{
		{
			ItagNo:   140,
			MimeType: `audio/mp4; codecs="mp4a.40.2"`,
			Bitrate:  128000,
		},
		{
			ItagNo:   251,
			MimeType: `audio/webm; codecs="opus"`,
			Bitrate:  160000,
		},
		{
			ItagNo:   250,
			MimeType: `audio/webm; codecs="opus"`,
			Bitrate:  64000,
		},
	}

	audioFormats := formats.Type("audio")
	if len(audioFormats) != 3 {
		t.Fatalf("expected 3 audio formats, got %d", len(audioFormats))
	}

	// Simulate selection logic from DownloadAudio
	var selected *ytlib.Format
	for i := range audioFormats {
		f := &audioFormats[i]
		if f.MimeType != "" && (strings.Contains(f.MimeType, "opus") || strings.Contains(f.MimeType, "webm")) {
			if selected == nil || f.Bitrate > selected.Bitrate {
				selected = f
			}
		}
	}

	if selected == nil {
		t.Fatal("no opus/webm format selected")
	}

	if selected.ItagNo != 251 {
		t.Errorf("expected itag 251 (highest bitrate opus), got %d", selected.ItagNo)
	}
}

func TestAudioStreamSelectionFallback(t *testing.T) {
	// Test fallback when no opus/webm available
	formats := ytlib.FormatList{
		{
			ItagNo:   140,
			MimeType: `audio/mp4; codecs="mp4a.40.2"`,
			Bitrate:  128000,
		},
		{
			ItagNo:   139,
			MimeType: `audio/mp4; codecs="mp4a.40.2"`,
			Bitrate:  48000,
		},
	}

	audioFormats := formats.Type("audio")

	var selected *ytlib.Format
	for i := range audioFormats {
		f := &audioFormats[i]
		if strings.Contains(f.MimeType, "opus") || strings.Contains(f.MimeType, "webm") {
			if selected == nil || f.Bitrate > selected.Bitrate {
				selected = f
			}
		}
	}

	// No opus/webm, fallback to highest bitrate
	if selected == nil {
		best := &audioFormats[0]
		for i := 1; i < len(audioFormats); i++ {
			if audioFormats[i].Bitrate > best.Bitrate {
				best = &audioFormats[i]
			}
		}
		selected = best
	}

	if selected.ItagNo != 140 {
		t.Errorf("expected itag 140 (highest bitrate fallback), got %d", selected.ItagNo)
	}
}

