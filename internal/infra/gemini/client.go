package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"google.golang.org/genai"

	"ai-news/internal/domain"
)

const (
	maxRetries     = 5
	baseBackoffSec = 2
)

// SelectionResult holds Gemini's article selection output for one category.
type SelectionResult struct {
	// ArticleID → rank (1-based, lower is higher priority)
	Ranks    map[int64]int
	// ArticleID → DJ narration summary
	Summaries map[int64]string
}

// Client wraps the Gemini generative AI client.
type Client struct {
	apiKey string
	model  string
}

// New creates a Client using the given API key and model name.
func New(apiKey, model string) *Client {
	return &Client{apiKey: apiKey, model: model}
}

// SelectAndSummarize calls Gemini to select the most newsworthy articles from
// the given list and generate DJ-style summaries for each selected article.
// articles must not be empty. articlesPerEpisode is the maximum number to select.
// summaryCharsPerArticle is the target character count per summary.
// Returns a SelectionResult with ranks and summaries keyed by article ID.
func (c *Client) SelectAndSummarize(
	ctx context.Context,
	articles []*domain.Article,
	category *domain.CategorySettings,
	articlesPerEpisode int,
	summaryCharsPerArticle int,
) (*SelectionResult, error) {
	prompt := buildPrompt(articles, category, articlesPerEpisode, summaryCharsPerArticle)

	var rawJSON string
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		rawJSON, err = c.callGemini(ctx, prompt)
		if err == nil {
			break
		}
		if !isRateLimit(err) || attempt == maxRetries {
			return nil, fmt.Errorf("gemini SelectAndSummarize: %w", err)
		}
		wait := backoffDuration(attempt)
		log.Printf("gemini: rate limit (attempt %d/%d), waiting %v: %v", attempt+1, maxRetries, wait, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	result, err := parseResponse(rawJSON, articles)
	if err != nil {
		return nil, fmt.Errorf("gemini parse response: %w", err)
	}
	return result, nil
}

// callGemini sends the prompt to the Gemini API and returns the text response.
func (c *Client) callGemini(ctx context.Context, prompt string) (string, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("create genai client: %w", err)
	}

	result, err := client.Models.GenerateContent(ctx, c.model,
		genai.Text(prompt),
		&genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
		},
	)
	if err != nil {
		return "", err
	}

	return result.Text(), nil
}

// buildPrompt constructs the Gemini prompt for article selection and summarisation.
func buildPrompt(
	articles []*domain.Article,
	cat *domain.CategorySettings,
	articlesPerEpisode int,
	summaryCharsPerArticle int,
) string {
	var sb strings.Builder
	sb.WriteString("あなたはラジオのDJです。以下のニュース記事の中から最もリスナーにとって価値のある記事を選んでください。\n\n")
	sb.WriteString(fmt.Sprintf("カテゴリ: %s\n", cat.DisplayName))
	sb.WriteString(fmt.Sprintf("選定数: 最大%d件\n", articlesPerEpisode))
	sb.WriteString(fmt.Sprintf("要約文字数: 各記事%d文字程度のDJナレーション原稿\n\n", summaryCharsPerArticle))

	sb.WriteString("記事一覧:\n")
	for i, a := range articles {
		sb.WriteString(fmt.Sprintf("[%d] ID=%d タイトル: %s\n", i+1, a.ID, a.Title))
		if a.RawContent != nil && *a.RawContent != "" {
			content := *a.RawContent
			if len([]rune(content)) > 500 {
				runes := []rune(content)
				content = string(runes[:500]) + "..."
			}
			sb.WriteString(fmt.Sprintf("    内容: %s\n", content))
		}
	}

	sb.WriteString("\n以下のJSON形式で返答してください。選定した記事のみを含めてください:\n")
	sb.WriteString(`{"selected": [{"id": <記事ID>, "rank": <重要度順位 1始まり>, "summary": "<DJナレーション原稿>"}]}`)
	sb.WriteString("\n\nrankは1が最も重要です。summaryは聴取者に向けたDJスタイルの原稿で、自然な日本語にしてください。")

	return sb.String()
}

// geminiResponse is the expected JSON structure from Gemini.
type geminiResponse struct {
	Selected []struct {
		ID      int64  `json:"id"`
		Rank    int    `json:"rank"`
		Summary string `json:"summary"`
	} `json:"selected"`
}

// parseResponse parses the Gemini JSON response into a SelectionResult.
func parseResponse(rawJSON string, articles []*domain.Article) (*SelectionResult, error) {
	// Strip markdown code fences if present
	rawJSON = strings.TrimSpace(rawJSON)
	if strings.HasPrefix(rawJSON, "```") {
		lines := strings.SplitN(rawJSON, "\n", 2)
		if len(lines) == 2 {
			rawJSON = lines[1]
		}
		rawJSON = strings.TrimSuffix(rawJSON, "```")
		rawJSON = strings.TrimSpace(rawJSON)
	}

	var resp geminiResponse
	if err := json.Unmarshal([]byte(rawJSON), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w (raw: %.200s)", err, rawJSON)
	}

	// Build a set of valid article IDs.
	validIDs := make(map[int64]bool, len(articles))
	for _, a := range articles {
		validIDs[a.ID] = true
	}

	result := &SelectionResult{
		Ranks:    make(map[int64]int, len(resp.Selected)),
		Summaries: make(map[int64]string, len(resp.Selected)),
	}
	for _, s := range resp.Selected {
		if !validIDs[s.ID] {
			log.Printf("gemini: unknown article ID %d in response, skipping", s.ID)
			continue
		}
		result.Ranks[s.ID] = s.Rank
		result.Summaries[s.ID] = s.Summary
	}
	return result, nil
}

// isRateLimit returns true if the error looks like a 429 / quota error.
func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate") ||
		strings.Contains(msg, "quota")
}

// backoffDuration computes jittered exponential backoff duration.
func backoffDuration(attempt int) time.Duration {
	exp := 1
	for i := 0; i < attempt; i++ {
		exp *= 2
	}
	base := time.Duration(baseBackoffSec*exp) * time.Second
	jitter := time.Duration(rand.Int63n(int64(base) / 2))
	return base + jitter
}
