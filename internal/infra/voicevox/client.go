package voicevox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	maxRetries     = 3
	retryBackoff   = 2 * time.Second
	requestTimeout = 5 * time.Minute
)

// Speaker represents a VOICEVOX speaker (style).
type Speaker struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Style string `json:"style"`
}

// Client is a VOICEVOX REST API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client for the given VOICEVOX base URL (e.g. "http://voicevox:50021").
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

// Synthesize converts text to WAV bytes using the specified speaker style ID.
// It retries up to maxRetries times on transient failures.
func (c *Client) Synthesize(ctx context.Context, text string, speakerID int, speedScale float64) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		wav, err := c.synthesize(ctx, text, speakerID, speedScale)
		if err == nil {
			return wav, nil
		}
		lastErr = err
		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoff):
			}
		}
	}
	return nil, fmt.Errorf("voicevox synthesize after %d attempts: %w", maxRetries, lastErr)
}

// synthesize performs one synthesis attempt: audio_query → synthesis.
func (c *Client) synthesize(ctx context.Context, text string, speakerID int, speedScale float64) ([]byte, error) {
	query, err := c.audioQuery(ctx, text, speakerID, speedScale)
	if err != nil {
		return nil, fmt.Errorf("audio_query: %w", err)
	}
	wav, err := c.synthesis(ctx, speakerID, query)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}
	return wav, nil
}

// audioQueryResponse is the minimal subset of the VOICEVOX audio_query JSON we need.
type audioQueryResponse map[string]interface{}

// audioQuery calls POST /audio_query and returns the modified query JSON.
func (c *Client) audioQuery(ctx context.Context, text string, speakerID int, speedScale float64) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/audio_query?text=%s&speaker=%d",
		c.baseURL,
		url.QueryEscape(text),
		speakerID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("audio_query HTTP %d: %s", resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Inject speedScale into the query JSON.
	var q map[string]interface{}
	if err := json.Unmarshal(body, &q); err != nil {
		return nil, fmt.Errorf("unmarshal audio_query: %w", err)
	}
	q["speedScale"] = speedScale
	modified, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("marshal modified query: %w", err)
	}
	return modified, nil
}

// synthesis calls POST /synthesis and returns WAV bytes.
func (c *Client) synthesis(ctx context.Context, speakerID int, query []byte) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/synthesis?speaker=%d", c.baseURL, speakerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("synthesis HTTP %d: %s", resp.StatusCode, body)
	}
	return io.ReadAll(resp.Body)
}

// Speakers returns the list of speakers from VOICEVOX, flattened to individual styles.
func (c *Client) Speakers(ctx context.Context) ([]Speaker, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/speakers", http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("speakers HTTP %d", resp.StatusCode)
	}

	// VOICEVOX speakers response: [{name, speaker_uuid, styles:[{id, name}]}]
	type style struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	type voicevoxSpeaker struct {
		Name   string  `json:"name"`
		Styles []style `json:"styles"`
	}
	var raw []voicevoxSpeaker
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode speakers: %w", err)
	}

	var speakers []Speaker
	for _, s := range raw {
		for _, st := range s.Styles {
			speakers = append(speakers, Speaker{
				ID:    st.ID,
				Name:  s.Name,
				Style: st.Name,
			})
		}
	}
	return speakers, nil
}
