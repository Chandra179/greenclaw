package router

import (
	"net/url"
	"regexp"
	"strings"
)

// YouTubeURLType represents the type of YouTube URL detected.
type YouTubeURLType int

const (
	YouTubeVideo    YouTubeURLType = iota + 1
	YouTubePlaylist
	YouTubeChannel
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

// IsYouTube checks whether the given URL is a YouTube URL and returns the type
// and extracted ID. For videos, ID is the video ID. For playlists, the playlist
// ID. For channels, the channel ID or handle.
func IsYouTube(rawURL string) (YouTubeURLType, string, bool) {
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
			return YouTubeVideo, id[:11], true
		}
		return 0, "", false
	}

	path := u.Path
	query := u.Query()

	// Playlist: youtube.com/playlist?list=...
	if path == "/playlist" || path == "/playlist/" {
		if list := query.Get("list"); list != "" {
			return YouTubePlaylist, list, true
		}
		return 0, "", false
	}

	// Watch: youtube.com/watch?v=...
	if path == "/watch" || path == "/watch/" {
		if v := query.Get("v"); len(v) >= 11 {
			// If there's also a list param, still treat as video
			return YouTubeVideo, v, true
		}
		return 0, "", false
	}

	// Shorts: youtube.com/shorts/<videoID>
	if m := shortsPattern.FindStringSubmatch(path); m != nil {
		return YouTubeVideo, m[1], true
	}

	// Embed: youtube.com/embed/<videoID>
	if m := embedPattern.FindStringSubmatch(path); m != nil {
		return YouTubeVideo, m[1], true
	}

	// Channel: youtube.com/channel/<channelID>
	if m := channelPattern.FindStringSubmatch(path); m != nil {
		return YouTubeChannel, m[1], true
	}

	// Handle: youtube.com/@handle
	if handlePattern.MatchString(path) {
		handle := strings.TrimPrefix(path, "/")
		// Strip trailing path segments
		if idx := strings.Index(handle, "/"); idx > 0 {
			handle = handle[:idx]
		}
		return YouTubeChannel, handle, true
	}

	return 0, "", false
}
