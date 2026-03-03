package navidrome

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

const (
	subsonicVersion = "1.16.1"
	subsonicClient  = "ai-news"
	saltLen         = 8
)

// Client is a Subsonic API client for Navidrome.
type Client struct {
	baseURL    string
	user       string
	pass       string
	httpClient *http.Client
}

// New creates a Navidrome client.
func New(baseURL, user, pass string) *Client {
	return &Client{
		baseURL: baseURL,
		user:    user,
		pass:    pass,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// StartScan triggers a Navidrome library scan via the Subsonic API.
func (c *Client) StartScan(ctx context.Context) error {
	salt := randomSalt(saltLen)
	token := md5Token(c.pass, salt)

	endpoint := fmt.Sprintf("%s/rest/startScan", c.baseURL)
	params := url.Values{
		"u": {c.user},
		"t": {token},
		"s": {salt},
		"v": {subsonicVersion},
		"c": {subsonicClient},
		"f": {"json"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), http.NoBody)
	if err != nil {
		return fmt.Errorf("navidrome startScan request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("navidrome startScan: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("navidrome startScan HTTP %d: %s", resp.StatusCode, body)
	}

	// Parse the Subsonic JSON envelope to check for errors.
	var envelope struct {
		SubsonicResponse struct {
			Status string `json:"status"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		// Non-fatal: can't parse but scan may have started.
		return nil
	}
	if envelope.SubsonicResponse.Status == "failed" && envelope.SubsonicResponse.Error != nil {
		e := envelope.SubsonicResponse.Error
		return fmt.Errorf("navidrome startScan subsonic error %d: %s", e.Code, e.Message)
	}
	return nil
}

// md5Token computes the Subsonic token: md5(password + salt).
func md5Token(password, salt string) string {
	h := md5.Sum([]byte(password + salt))
	return fmt.Sprintf("%x", h)
}

// randomSalt generates a random alphanumeric salt string of length n.
func randomSalt(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
