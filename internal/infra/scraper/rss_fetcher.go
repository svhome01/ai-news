package scraper

import (
	"context"
	"strings"

	"github.com/mmcdole/gofeed"

	"ai-news/internal/domain"
)

// RSSFetcher retrieves articles from RSS or Atom feeds using gofeed.
type RSSFetcher struct{}

// Fetch parses the RSS/Atom feed at src.URL and returns article candidates.
// Thumbnail URLs are extracted in priority order:
//  1. item.Image.URL (RSS <image> element)
//  2. <enclosure type="image/*"> URL
//  3. <media:thumbnail url="…"> attribute
//  4. <media:content medium="image" url="…"> attribute
func (f *RSSFetcher) Fetch(ctx context.Context, src *domain.Source) ([]FetchResult, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(src.URL, ctx)
	if err != nil {
		return nil, err
	}

	results := make([]FetchResult, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item.Link == "" {
			continue
		}
		r := FetchResult{
			Title: strings.TrimSpace(item.Title),
			URL:   item.Link,
		}
		if r.Title == "" {
			r.Title = item.Link
		}

		// Collect raw content for Gemini summarisation.
		if item.Content != "" {
			r.RawContent = item.Content
		} else if item.Description != "" {
			r.RawContent = item.Description
		}

		// Resolve thumbnail URL (best effort).
		r.RemoteThumb = resolveFeedThumb(item)

		results = append(results, r)
	}
	return results, nil
}

// resolveFeedThumb extracts the best available thumbnail URL from a feed item.
func resolveFeedThumb(item *gofeed.Item) string {
	// Priority 1: <image> element
	if item.Image != nil && item.Image.URL != "" {
		return item.Image.URL
	}

	// Priority 2: enclosure with image MIME type
	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}

	// Priority 3: <media:thumbnail url="…">
	if media, ok := item.Extensions["media"]; ok {
		if thumbs, ok := media["thumbnail"]; ok {
			for _, t := range thumbs {
				if u := t.Attrs["url"]; u != "" {
					return u
				}
			}
		}
		// Priority 4: <media:content medium="image" url="…">
		if contents, ok := media["content"]; ok {
			for _, c := range contents {
				if (c.Attrs["medium"] == "image" || strings.HasPrefix(c.Attrs["type"], "image/")) && c.Attrs["url"] != "" {
					return c.Attrs["url"]
				}
			}
		}
	}

	return ""
}
