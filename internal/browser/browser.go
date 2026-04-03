package browser

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"

	"greenclaw/internal/store"
)

// BrowserFetcher abstracts browser-based page fetching for testability.
type BrowserFetcher interface {
	FetchPage(ctx context.Context, url string) (*store.Result, error)
	Close()
}

// Pool manages a pool of browser instances with automatic recycling.
type Pool struct {
	mu           sync.Mutex
	browser      *rod.Browser
	pageCount    int
	recycleAfter int
}

// NewPool creates a browser pool that recycles after the given number of pages.
func NewPool(recycleAfter int) *Pool {
	return &Pool{
		recycleAfter: recycleAfter,
	}
}

func (p *Pool) ensureBrowser() (*rod.Browser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.browser != nil && p.pageCount < p.recycleAfter {
		p.pageCount++
		return p.browser, nil
	}

	// Recycle: close old browser if exists
	if p.browser != nil {
		log.Println("[browser] recycling browser after", p.pageCount, "pages")
		p.browser.MustClose()
		p.browser = nil
	}

	u, err := launcher.New().
		Set("disable-gpu").
		Set("disable-dev-shm-usage").
		Set("no-sandbox").
		Launch()
	if err != nil {
		return nil, fmt.Errorf("launching browser: %w", err)
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("connecting to browser: %w", err)
	}

	p.browser = b
	p.pageCount = 1
	return p.browser, nil
}

// FetchPage uses a headless browser with stealth to render and extract a page.
func (p *Pool) FetchPage(ctx context.Context, url string) (*store.Result, error) {
	browser, err := p.ensureBrowser()
	if err != nil {
		return nil, err
	}

	page, err := stealth.Page(browser)
	if err != nil {
		return nil, fmt.Errorf("creating stealth page: %w", err)
	}
	defer page.Close()

	setupIntercept(page)

	// Set viewport to common desktop resolution
	page.MustSetViewport(1920, 1080, 1, false)

	err = rod.Try(func() {
		page.Context(ctx).MustNavigate(url).MustWaitStable()
	})
	if err != nil {
		return nil, fmt.Errorf("navigating to page: %w", err)
	}

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("getting page HTML: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parsing rendered HTML: %w", err)
	}

	r := &store.Result{
		URL:       url,
		FetchedAt: time.Now(),
	}

	r.Title = strings.TrimSpace(doc.Find("title").First().Text())

	doc.Find("meta[name=description]").Each(func(_ int, s *goquery.Selection) {
		if content, exists := s.Attr("content"); exists {
			r.Description = strings.TrimSpace(content)
		}
	})

	doc.Find("script, style, noscript").Remove()
	r.Text = strings.TrimSpace(doc.Find("body").Text())

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

// Close shuts down the browser pool.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.browser != nil {
		p.browser.MustClose()
		p.browser = nil
	}
}
