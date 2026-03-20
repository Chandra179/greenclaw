package youtube

import (
	"strings"
	"testing"
)

func TestToSRT(t *testing.T) {
	entries := []TimedEntry{
		{Start: 0.0, Dur: 2.5, Text: "Hello world"},
		{Start: 2.5, Dur: 3.0, Text: "Second line"},
	}

	got := toSRT(entries)

	if !strings.Contains(got, "1\n00:00:00,000 --> 00:00:02,500\nHello world") {
		t.Errorf("SRT output missing first entry:\n%s", got)
	}
	if !strings.Contains(got, "2\n00:00:02,500 --> 00:00:05,500\nSecond line") {
		t.Errorf("SRT output missing second entry:\n%s", got)
	}
}

func TestToVTT(t *testing.T) {
	entries := []TimedEntry{
		{Start: 0.0, Dur: 2.5, Text: "Hello world"},
		{Start: 62.5, Dur: 1.0, Text: "Over a minute"},
	}

	got := toVTT(entries)

	if !strings.HasPrefix(got, "WEBVTT\n\n") {
		t.Errorf("VTT output should start with WEBVTT header:\n%s", got)
	}
	if !strings.Contains(got, "00:00:00.000 --> 00:00:02.500\nHello world") {
		t.Errorf("VTT output missing first entry:\n%s", got)
	}
	if !strings.Contains(got, "00:01:02.500 --> 00:01:03.500\nOver a minute") {
		t.Errorf("VTT output missing second entry:\n%s", got)
	}
}

func TestToTTML(t *testing.T) {
	entries := []TimedEntry{
		{Start: 0.0, Dur: 1.0, Text: "Hello"},
	}

	got := toTTML(entries)

	if !strings.Contains(got, `<?xml version="1.0"`) {
		t.Error("TTML should have XML declaration")
	}
	if !strings.Contains(got, `<tt xmlns="http://www.w3.org/ns/ttml">`) {
		t.Error("TTML should have tt root element")
	}
	if !strings.Contains(got, `begin="00:00:00.000"`) {
		t.Errorf("TTML missing begin attribute:\n%s", got)
	}
	if !strings.Contains(got, `end="00:00:01.000"`) {
		t.Errorf("TTML missing end attribute:\n%s", got)
	}
}

func TestFormatSRTTime(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0.0, "00:00:00,000"},
		{1.5, "00:00:01,500"},
		{62.123, "00:01:02,123"},
		{3661.0, "01:01:01,000"},
	}

	for _, tt := range tests {
		got := formatSRTTime(tt.seconds)
		if got != tt.want {
			t.Errorf("formatSRTTime(%f) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestParseSubtitleFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    SubtitleFormat
		wantErr bool
	}{
		{"srt", FormatSRT, false},
		{"SRT", FormatSRT, false},
		{"vtt", FormatVTT, false},
		{"webvtt", FormatVTT, false},
		{"ttml", FormatTTML, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		got, err := ParseSubtitleFormat(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSubtitleFormat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseSubtitleFormat(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
