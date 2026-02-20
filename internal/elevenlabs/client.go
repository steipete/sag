package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Client talks to the ElevenLabs HTTP API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient returns a Client configured with the given API key and base URL.
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.elevenlabs.io"
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Voice represents a voice entry returned by ElevenLabs.
type Voice struct {
	VoiceID     string            `json:"voice_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels,omitempty"`
	PreviewURL  string            `json:"preview_url"`
}

type listVoicesResponse struct {
	Voices []Voice `json:"voices"`
	Next   *string `json:"next_page_token,omitempty"`
}

type listVoicesV2Response struct {
	Voices        []Voice `json:"voices"`
	HasMore       bool    `json:"has_more"`
	NextPageToken *string `json:"next_page_token,omitempty"`
}

// ListVoices fetches available voices.
func (c *Client) ListVoices(ctx context.Context) ([]Voice, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "/v1/voices")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list voices failed: %s", resp.Status)
	}

	var body listVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Voices, nil
}

// SearchVoices finds voices using the API's search query parameter.
func (c *Client) SearchVoices(ctx context.Context, search string, limit int) ([]Voice, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return c.ListVoices(ctx)
	}

	pageSize := 100
	if limit > 0 && limit < pageSize {
		pageSize = limit
	}

	voices := make([]Voice, 0, pageSize)
	var nextPageToken *string
	for {
		u, err := url.Parse(c.baseURL)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, "/v2/voices")
		q := u.Query()
		q.Set("search", search)
		q.Set("page_size", fmt.Sprint(pageSize))
		q.Set("include_total_count", "false")
		if nextPageToken != nil && *nextPageToken != "" {
			q.Set("next_page_token", *nextPageToken)
		}
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("xi-api-key", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode >= 400 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("search voices failed: %s", resp.Status)
		}

		var body listVoicesV2Response
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		_ = resp.Body.Close()
		voices = append(voices, body.Voices...)
		if limit > 0 && len(voices) >= limit {
			return voices[:limit], nil
		}
		if !body.HasMore || body.NextPageToken == nil || *body.NextPageToken == "" {
			return voices, nil
		}
		nextPageToken = body.NextPageToken
	}
}

// GetVoice fetches metadata for a specific voice.
func (c *Client) GetVoice(ctx context.Context, voiceID string) (Voice, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return Voice{}, err
	}
	u.Path = path.Join(u.Path, "/v1/voices", voiceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Voice{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Voice{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return Voice{}, fmt.Errorf("get voice failed: %s", resp.Status)
	}

	var voice Voice
	if err := json.NewDecoder(resp.Body).Decode(&voice); err != nil {
		return Voice{}, err
	}
	return voice, nil
}

// TTSRequest configures a text-to-speech request payload.
type TTSRequest struct {
	Text                   string         `json:"text"`
	ModelID                string         `json:"model_id,omitempty"`
	VoiceSettings          *VoiceSettings `json:"voice_settings,omitempty"`
	OutputFormat           string         `json:"output_format,omitempty"`
	Seed                   *uint32        `json:"seed,omitempty"`
	ApplyTextNormalization string         `json:"apply_text_normalization,omitempty"`
	LanguageCode           string         `json:"language_code,omitempty"`
}

// VoiceSettings tunes synthesis parameters for a request.
type VoiceSettings struct {
	Stability       *float64 `json:"stability,omitempty"`
	SimilarityBoost *float64 `json:"similarity_boost,omitempty"`
	Style           *float64 `json:"style,omitempty"`
	UseSpeakerBoost *bool    `json:"use_speaker_boost,omitempty"`
	Speed           *float64 `json:"speed,omitempty"`
}

// StreamTTS requests streaming audio for text-to-speech.
func (c *Client) StreamTTS(ctx context.Context, voiceID string, payload TTSRequest, latency int) (io.ReadCloser, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "/v1/text-to-speech", voiceID, "stream")
	q := u.Query()
	if latency > 0 {
		q.Set("optimize_streaming_latency", fmt.Sprint(latency))
	}
	if payload.OutputFormat != "" {
		q.Set("output_format", payload.OutputFormat)
		payload.OutputFormat = "" // don't send in body
	}
	u.RawQuery = q.Encode()

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer func() {
			_ = resp.Body.Close()
		}()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stream TTS failed: %s: %s", resp.Status, string(b))
	}
	return resp.Body, nil
}

// ConvertTTS downloads the full audio before returning.
func (c *Client) ConvertTTS(ctx context.Context, voiceID string, payload TTSRequest) ([]byte, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "/v1/text-to-speech", voiceID)
	q := u.Query()
	if payload.OutputFormat != "" {
		q.Set("output_format", payload.OutputFormat)
		payload.OutputFormat = ""
	}
	u.RawQuery = q.Encode()

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("convert TTS failed: %s: %s", resp.Status, string(b))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}
