package scraper

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"ai-news/internal/domain"
)

// HTTPFetcher retrieves article links from a static HTML page using goquery.
// If src.CSSSelector is set, it is used to find article containers;
// otherwise all <a> tags on the page are returned.
// The page-level og:image serves as a shared thumbnail URL for all found articles.
type HTTPFetcher struct{}

// Fetch downloads the source URL, parses the HTML, and returns article candidates.
func (f *HTTPFetcher) Fetch(ctx context.Context, src *domain.Source) ([]FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ai-news/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	base, _ := url.Parse(src.URL)

	// Page-level og:image for use as a shared thumbnail fallback.
	pageThumb, _ := doc.Find("meta[property='og:image']").Attr("content")

	selector := "a"
	if src.CSSSelector != nil && *src.CSSSelector != "" {
		selector = *src.CSSSelector + " a, " + *src.CSSSelector
	}

	seen := make(map[string]bool)
	var results []FetchResult

	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		// Resolve the link: either the element itself or an ancestor <a>.
		var anchor *goquery.Selection
		if goquery.NodeName(s) == "a" {
			anchor = s
		} else {
			anchor = s.Find("a").First()
			if anchor.Length() == 0 {
				anchor = s.Closest("a")
			}
		}
		if anchor.Length() == 0 {
			return
		}

		href, exists := anchor.Attr("href")
		if !exists || href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			return
		}

		// Resolve relative URLs against the base URL.
		absURL := resolveURL(base, href)
		if absURL == "" || seen[absURL] {
			return
		}
		seen[absURL] = true

		title := strings.TrimSpace(anchor.Text())
		if title == "" {
			title = absURL
		}

		// Try to find a per-article thumbnail in an img tag within the container.
		thumb := pageThumb
		if src.CSSSelector != nil && *src.CSSSelector != "" {
			container := s.Closest(*src.CSSSelector)
			if container.Length() == 0 {
				container = s
			}
			if imgSrc, ok := container.Find("img").First().Attr("src"); ok && imgSrc != "" {
				if abs := resolveURL(base, imgSrc); abs != "" {
					thumb = abs
				}
			}
		}

		results = append(results, FetchResult{
			Title:       title,
			URL:         absURL,
			RemoteThumb: thumb,
		})
	})

	return results, nil
}

// resolveURL resolves href against base. Returns "" on error.
func resolveURL(base *url.URL, href string) string {
	if base == nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}
