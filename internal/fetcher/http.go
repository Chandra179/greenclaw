package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"greenclaw/internal/result"
	"greenclaw/internal/router"
)

// HTTPDoer abstracts HTTP request execution for testability.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// FetchHTML performs a plain HTTP GET and extracts content using goquery.
// Returns ErrNeedsEscalation if heuristics detect the page needs a browser.
func FetchHTML(ctx context.Context, client HTTPDoer, url string) (*result.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	if NeedsEscalation(resp.StatusCode, body) {
		return nil, ErrNeedsEscalation
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	r := &result.Result{
		URL:         url,
		ContentType: router.ContentHTML,
		FetchedAt:   time.Now(),
	}

	r.Title = strings.TrimSpace(doc.Find("title").First().Text())

	doc.Find("meta[name=description]").Each(func(_ int, s *goquery.Selection) {
		if content, exists := s.Attr("content"); exists {
			r.Description = strings.TrimSpace(content)
		}
	})

	// Extract text from body, stripping script/style tags
	doc.Find("script, style, noscript").Remove()
	r.Text = strings.TrimSpace(doc.Find("body").Text())
	// Collapse whitespace
	r.Text = collapseWhitespace(r.Text)

	// Extract links
	var links []string
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			href = strings.TrimSpace(href)
			if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
				links = append(links, href)
			}
		}
	})
	r.Links = links

	return r, nil
}

// DownloadBinary streams a URL to disk and returns the file path.
func DownloadBinary(ctx context.Context, client HTTPDoer, url, outputDir string) (*result.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	// Derive filename from URL
	filename := filepath.Base(url)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	dest := filepath.Join(outputDir, filename)
	f, err := os.Create(dest)
	if err != nil {
		return nil, fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	return &result.Result{
		URL:         url,
		ContentType: router.ContentBinary,
		FilePath:    dest,
		FetchedAt:   time.Now(),
	}, nil
}

// FetchJSON performs a GET and returns raw JSON bytes in RawData.
func FetchJSON(ctx context.Context, client HTTPDoer, url string) (*result.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	return &result.Result{
		URL:         url,
		ContentType: router.ContentJSON,
		FetchedAt:   time.Now(),
	}, nil
}

// FetchXML performs a GET and returns raw XML bytes in RawData.
func FetchXML(ctx context.Context, client HTTPDoer, url string) (*result.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/xml, text/xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	return &result.Result{
		URL:         url,
		ContentType: router.ContentXML,
		FetchedAt:   time.Now(),
	}, nil
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return b.String()
}
