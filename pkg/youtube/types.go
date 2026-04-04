package youtube

// VideoMetadata holds basic metadata for a YouTube video.
type VideoMetadata struct {
	VideoID     string
	Title       string
	Description string
	Duration    string
	ViewCount   int
	UploadDate  string
	ChannelName string
	ChannelID   string
	Captions    []CaptionTrack
}

// PlaylistItem represents a single video in a playlist.
type PlaylistItem struct {
	VideoID string
	Title   string
	Index   int
}

// CaptionTrack represents a single caption/subtitle track.
type CaptionTrack struct {
	LanguageCode string
	Text         string
}

// TimedEntry represents a single timed text entry from YouTube's caption XML.
type TimedEntry struct {
	Start float64 `xml:"start,attr"`
	Dur   float64 `xml:"dur,attr"`
	Text  string  `xml:",chardata"`
}

type timedTextXML struct {
	Entries []TimedEntry `xml:"text"`
}
