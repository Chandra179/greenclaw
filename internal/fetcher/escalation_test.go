package fetcher

import "testing"

func TestNeedsEscalation(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{
			name:       "403 with body",
			statusCode: 403,
			body:       "<html><body>Access Denied</body></html>",
			want:       true,
		},
		{
			name:       "503 with body",
			statusCode: 503,
			body:       "<html><body>Service Unavailable</body></html>",
			want:       true,
		},
		{
			name:       "200 normal page",
			statusCode: 200,
			body:       "<html><head><title>Hello</title></head><body><p>This is a normal page with enough content to pass the heuristic check.</p></body></html>",
			want:       false,
		},
		{
			name:       "cloudflare challenge",
			statusCode: 200,
			body:       `<html><body><div id="cf-browser-verification">Please wait...</div></body></html>`,
			want:       true,
		},
		{
			name:       "cloudflare chl marker",
			statusCode: 200,
			body:       `<html><body><input name="__cf_chl_tk" /></body></html>`,
			want:       true,
		},
		{
			name:       "js required noscript",
			statusCode: 200,
			body:       `<html><body><noscript>You need to enable JavaScript to run this app.</noscript><div id="root"></div></body></html>`,
			want:       true,
		},
		{
			name:       "captcha challenge",
			statusCode: 200,
			body:       `<html><body><div class="captcha-container"><p>Complete this challenge to continue</p></div></body></html>`,
			want:       true,
		},
		{
			name:       "js shell - large html small text",
			statusCode: 200,
			body:       "<html><head><script src='app.js'></script><script src='vendor.js'></script><link rel='stylesheet' href='app.css'/><meta charset='utf-8'/><title>App</title></head><body><div id='root'></div><script>window.__INITIAL_STATE__={}</script><script src='chunk1.js'></script><script src='chunk2.js'></script><script src='chunk3.js'></script><script src='chunk4.js'></script><script src='chunk5.js'></script><script src='chunk6.js'></script><script src='chunk7.js'></script><script src='chunk8.js'></script><script src='chunk9.js'></script><script src='chunk10.js'></script></body></html>",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsEscalation(tt.statusCode, []byte(tt.body))
			if got != tt.want {
				t.Errorf("NeedsEscalation(%d, ...) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}
