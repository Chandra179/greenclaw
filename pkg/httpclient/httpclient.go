package httpclient

import (
	"net/http"
	"time"
)

// New returns an *http.Client with the given timeout and a browser-like User-Agent
// applied via a RoundTripper wrapper.
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &userAgentTransport{base: http.DefaultTransport},
	}
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	return t.base.RoundTrip(req)
}
