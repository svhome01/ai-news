package domain

// FetchMethod is the method used to retrieve articles from a source.
type FetchMethod string

const (
	FetchMethodRSS        FetchMethod = "rss"
	FetchMethodHTTP       FetchMethod = "http"
	FetchMethodPlaywright FetchMethod = "playwright"
)

// Source is a news source configuration.
type Source struct {
	ID          int64
	Name        string
	URL         string
	Category    string
	FetchMethod FetchMethod
	CSSSelector *string // used for http/playwright methods
	Enabled     bool
	CreatedAt   string
	UpdatedAt   string
}
