package youtube

import (
	"context"

	pkgyt "greenclaw/pkg/youtube"

	"greenclaw/internal/result"

	ytlib "github.com/kkdai/youtube/v2"
)

// TimedEntry is re-exported from pkg/youtube for callers that need timed data.
type TimedEntry = pkgyt.TimedEntry

// GetTranscript fetches a single caption track by language code.
func (c *Client) GetTranscript(ctx context.Context, video *ytlib.Video, langCode string) (*result.CaptionTrack, []TimedEntry, error) {
	track, entries, err := c.pkg.GetTranscript(ctx, video, langCode)
	if err != nil {
		return nil, nil, err
	}

	cap := &result.CaptionTrack{
		LanguageCode: track.LanguageCode,
		Text:         track.Text,
	}
	return cap, entries, nil
}
