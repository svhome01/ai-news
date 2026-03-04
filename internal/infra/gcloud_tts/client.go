package gcloud_tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	apiEndpoint    = "https://texttospeech.googleapis.com/v1/text:synthesize"
	defaultVoice   = "ja-JP-Neural2-B"
	defaultLangCode = "ja-JP"
	requestTimeout = 30 * time.Second
)

// Client is a Google Cloud Text-to-Speech REST client.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// New creates a Client with the given API key.
func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// Synthesize sends text to the Cloud TTS API and returns MP3 bytes.
// voice is a Cloud TTS voice name (e.g. "ja-JP-Neural2-B"); empty → defaultVoice.
// speedScale maps to speakingRate (0.25–4.0); ≤0 treated as 1.0.
func (c *Client) Synthesize(ctx context.Context, text, voice string, speedScale float64) ([]byte, error) {
	if voice == "" {
		voice = defaultVoice
	}
	if speedScale <= 0 {
		speedScale = 1.0
	}

	reqBody := map[string]any{
		"input": map[string]string{
			"text": text,
		},
		"voice": map[string]string{
			"languageCode": defaultLangCode,
			"name":         voice,
		},
		"audioConfig": map[string]any{
			"audioEncoding": "MP3",
			"speakingRate":  speedScale,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiEndpoint+"?key="+c.apiKey,
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gcloud TTS status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	mp3Bytes, err := base64.StdEncoding.DecodeString(result.AudioContent)
	if err != nil {
		return nil, fmt.Errorf("decode base64 audio: %w", err)
	}
	return mp3Bytes, nil
}
