package fetcher

import (
	"bytes"
	"errors"
	"strings"
)

// ErrNeedsEscalation is returned when heuristics determine a page needs
// browser rendering to extract content.
var ErrNeedsEscalation = errors.New("page needs browser escalation")

// NeedsEscalation checks whether an HTTP response indicates the page
// requires a full browser to extract content.
func NeedsEscalation(statusCode int, body []byte) bool {
	// Blocked status codes with HTML body
	if (statusCode == 403 || statusCode == 503) && len(body) > 0 {
		return true
	}

	lower := bytes.ToLower(body)

	// Cloudflare challenge markers
	if bytes.Contains(lower, []byte("cf-browser-verification")) ||
		bytes.Contains(lower, []byte("__cf_chl_")) ||
		bytes.Contains(lower, []byte("cf-challenge-running")) ||
		bytes.Contains(lower, []byte("challenge-platform")) {
		return true
	}

	// Generic CAPTCHA markers
	if bytes.Contains(lower, []byte("captcha")) && bytes.Contains(lower, []byte("challenge")) {
		return true
	}

	// JS-required patterns: page is essentially empty without JS
	if bytes.Contains(lower, []byte("<noscript>")) {
		bodyStr := strings.ToLower(string(body))
		// If there's a noscript tag telling user to enable JS, and the
		// actual body content is very short, likely JS-rendered
		if strings.Contains(bodyStr, "enable javascript") ||
			strings.Contains(bodyStr, "javascript is required") ||
			strings.Contains(bodyStr, "you need to enable javascript") {
			return true
		}
	}

	// Very short body after stripping tags — likely a JS shell
	stripped := stripTags(body)
	trimmed := bytes.TrimSpace(stripped)
	if len(body) > 500 && len(trimmed) < 100 {
		return true
	}

	return false
}

// stripTags removes HTML tags, returning only text content.
func stripTags(b []byte) []byte {
	var out bytes.Buffer
	inTag := false
	for _, c := range b {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteByte(c)
		}
	}
	return out.Bytes()
}
