package youtube

import (
	"testing"
)

func TestParseTimedTextXML(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<transcript>
  <text start="0.0" dur="2.5">Hello world</text>
  <text start="2.5" dur="3.0">This is a test</text>
  <text start="5.5" dur="1.5">with &amp; special &lt;chars&gt;</text>
</transcript>`)

	entries, err := parseTimedTextXML(xmlData)
	if err != nil {
		t.Fatalf("parseTimedTextXML() error = %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	if entries[0].Start != 0.0 {
		t.Errorf("entry[0].Start = %f, want 0.0", entries[0].Start)
	}
	if entries[0].Dur != 2.5 {
		t.Errorf("entry[0].Dur = %f, want 2.5", entries[0].Dur)
	}
	if entries[0].Text != "Hello world" {
		t.Errorf("entry[0].Text = %q, want %q", entries[0].Text, "Hello world")
	}

	if entries[2].Text != "with & special <chars>" {
		t.Errorf("entry[2].Text = %q, want %q", entries[2].Text, "with & special <chars>")
	}
}

func TestEntriesToPlainText(t *testing.T) {
	entries := []TimedEntry{
		{Start: 0, Dur: 1, Text: "Hello"},
		{Start: 1, Dur: 1, Text: "world"},
		{Start: 2, Dur: 1, Text: "test"},
	}

	got := entriesToPlainText(entries)
	want := "Hello world test"
	if got != want {
		t.Errorf("entriesToPlainText() = %q, want %q", got, want)
	}
}
