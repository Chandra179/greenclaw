package constant

// HTTPContentType represents the detected content type of a URL.
type HTTPContentType string

const (
	HTTPContentHTML   HTTPContentType = "html"
	HTTPContentJSON   HTTPContentType = "json"
	HTTPContentXML    HTTPContentType = "xml"
	HTTPContentBinary HTTPContentType = "binary"
)

// ContentType represents the detected content type of a URL.
type YoutubeContentType string

// Content type constants for YouTube URLs.
const (
	YoutubeContentVideo    YoutubeContentType = "youtube_video"
	YoutubeContentPlaylist YoutubeContentType = "youtube_playlist"
	YoutubeContentChannel  YoutubeContentType = "youtube_channel"
)
