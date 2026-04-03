package youtube

import (
	"net/url"
	"regexp"
	"strings"

	"greenclaw/internal/router"
)

// URLType represents the type of YouTube URL detected.
type URLType int

const (
	VideoURL    URLType = iota + 1
	PlaylistURL
	ChannelURL
)

// Content type constants for YouTube URLs.
const (
	ContentVideo    router.ContentType = "youtube_video"
	ContentPlaylist router.ContentType = "youtube_playlist"
	ContentChannel  router.ContentType = "youtube_channel"
)

var (
	ytHosts = map[string]bool{
		"youtube.com":     true,
		"www.youtube.com": true,
		"m.youtube.com":   true,
		"youtu.be":        true,
	}

	shortsPattern  = regexp.MustCompile(`^/shorts/([a-zA-Z0-9_-]{11})`)
	embedPattern   = regexp.MustCompile(`^/embed/([a-zA-Z0-9_-]{11})`)
	handlePattern  = regexp.MustCompile(`^/@[\w.-]+`)
	channelPattern = regexp.MustCompile(`^/channel/(UC[\w-]{22})`)
)

// Detect checks whether the given URL is a YouTube URL and returns the type
// and extracted ID. For videos, ID is the video ID. For playlists, the playlist
// ID. For channels, the channel ID or handle.
func Detect(rawURL string) (URLType, string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, "", false
	}

	host := strings.ToLower(u.Hostname())
	if !ytHosts[host] {
		return 0, "", false
	}

	// Short URL: youtu.be/<videoID>
	if host == "youtu.be" {
		id := strings.TrimPrefix(u.Path, "/")
		if len(id) >= 11 {
			return VideoURL, id[:11], true
		}
		return 0, "", false
	}

	path := u.Path
	query := u.Query()

	// Playlist: youtube.com/playlist?list=...
	if path == "/playlist" || path == "/playlist/" {
		if list := query.Get("list"); list != "" {
			return PlaylistURL, list, true
		}
		return 0, "", false
	}

	// Watch: youtube.com/watch?v=...
	if path == "/watch" || path == "/watch/" {
		if v := query.Get("v"); len(v) >= 11 {
			return VideoURL, v, true
		}
		return 0, "", false
	}

	// Shorts: youtube.com/shorts/<videoID>
	if m := shortsPattern.FindStringSubmatch(path); m != nil {
		return VideoURL, m[1], true
	}

	// Embed: youtube.com/embed/<videoID>
	if m := embedPattern.FindStringSubmatch(path); m != nil {
		return VideoURL, m[1], true
	}

	// Channel: youtube.com/channel/<channelID>
	if m := channelPattern.FindStringSubmatch(path); m != nil {
		return ChannelURL, m[1], true
	}

	// Handle: youtube.com/@handle
	if handlePattern.MatchString(path) {
		handle := strings.TrimPrefix(path, "/")
		if idx := strings.Index(handle, "/"); idx > 0 {
			handle = handle[:idx]
		}
		return ChannelURL, handle, true
	}

	return 0, "", false
}
