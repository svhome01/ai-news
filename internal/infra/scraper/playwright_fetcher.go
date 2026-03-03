package scraper

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"

	playwright "github.com/playwright-community/playwright-go"

	"ai-news/internal/domain"
)

// PlaywrightFetcher uses a remote Chromium instance (via CDP) to fetch JS-rendered pages.
// It connects to the playwright-chrome container at the configured CDP endpoint.
//
// The playwright server process is initialised once (sync.Once) to avoid the startup
// overhead on every scrape cycle.  The browser connection is re-established per Fetch
// call so that a Chrome restart does not leave the fetcher in a broken state.
type PlaywrightFetcher struct {
	cdpEndpoint string

	once sync.Once
	pw   *playwright.Playwright
	pwErr error
}

// NewPlaywrightFetcher creates a PlaywrightFetcher for the given CDP endpoint.
func NewPlaywrightFetcher(cdpEndpoint string) *PlaywrightFetcher {
	return &PlaywrightFetcher{cdpEndpoint: cdpEndpoint}
}

// initOnce starts the Playwright server process exactly once.
// If it fails, all subsequent Fetch calls return the stored error.
func (f *PlaywrightFetcher) initOnce() error {
	f.once.Do(func() {
		pw, err := playwright.Run()
		if err != nil {
			f.pwErr = fmt.Errorf("playwright run: %w", err)
			return
		}
		f.pw = pw
	})
	return f.pwErr
}

// Fetch navigates to src.URL using a remote Chromium instance and extracts article links.
// The CSS selector from src.CSSSelector is used to target article containers (falls back
// to all <a> tags).  The page og:image is returned as the thumbnail for all articles.
func (f *PlaywrightFetcher) Fetch(ctx context.Context, src *domain.Source) ([]FetchResult, error) {
	if err := f.initOnce(); err != nil {
		return nil, err
	}

	browser, err := f.pw.Chromium.ConnectOverCDP(f.cdpEndpoint)
	if err != nil {
		return nil, fmt.Errorf("playwright connect CDP: %w", err)
	}
	defer func() {
		if err := browser.Close(); err != nil {
			log.Printf("playwright: close browser: %v", err)
		}
	}()

	bctx, err := browser.NewContext()
	if err != nil {
		return nil, fmt.Errorf("playwright new context: %w", err)
	}
	defer bctx.Close()

	page, err := bctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("playwright new page: %w", err)
	}
	defer page.Close()

	if _, err := page.Goto(src.URL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30_000),
	}); err != nil {
		return nil, fmt.Errorf("playwright goto %q: %w", src.URL, err)
	}

	// Extract og:image for use as a thumbnail fallback.
	pageThumb := ""
	if raw, err := page.Evaluate(`() => { const m = document.querySelector('meta[property="og:image"]'); return m ? m.content : ''; }`); err == nil {
		if s, ok := raw.(string); ok {
			pageThumb = s
		}
	}

	// Build JS snippet to find article links.
	selector := "a"
	if src.CSSSelector != nil && *src.CSSSelector != "" {
		selector = *src.CSSSelector + " a, " + *src.CSSSelector
	}

	jsExpr := fmt.Sprintf(`() => {
		const seen = new Set();
		const results = [];
		document.querySelectorAll(%q).forEach(el => {
			const a = el.tagName === 'A' ? el : el.querySelector('a');
			if (!a || !a.href || seen.has(a.href)) return;
			seen.add(a.href);
			results.push({ title: (a.textContent || '').trim(), url: a.href });
		});
		return results;
	}`, selector)

	raw, err := page.Evaluate(jsExpr)
	if err != nil {
		return nil, fmt.Errorf("playwright evaluate: %w", err)
	}

	base, _ := url.Parse(src.URL)

	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("playwright: unexpected evaluate result type")
	}

	var results []FetchResult
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		link, _ := m["url"].(string)
		title, _ := m["title"].(string)
		if link == "" || strings.HasPrefix(link, "#") {
			continue
		}
		absURL := resolveURL(base, link)
		if absURL == "" {
			continue
		}
		if title == "" {
			title = absURL
		}
		results = append(results, FetchResult{
			Title:       title,
			URL:         absURL,
			RemoteThumb: pageThumb,
		})
	}
	return results, nil
}
