// Package deepgram provides streaming Aura text-to-speech over HTTP.
package deepgram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/steipete/sag/internal/tts"
)

// DefaultVoice is the English Aura-2 voice used without an explicit selection.
const DefaultVoice = "aura-2-thalia-en"

const maxAudioBytes = 64 << 20

// Client uses a dedicated API key; baseURL includes the API version path.
type Client struct {
	apiKey, baseURL string
	httpClient      *http.Client
}

// NewClient creates a Deepgram client with a reusable HTTP transport.
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.deepgram.com/v1"
	}
	return &Client{apiKey: apiKey, baseURL: baseURL, httpClient: &http.Client{}}
}

func (c *Client) request(ctx context.Context, method, route string, query url.Values, body io.Reader) (*http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("deepgram: base URL must use HTTP(S), with a host and no credentials, query, or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/") + route
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("deepgram: authentication rejected (HTTP 401): replace DEEPGRAM_API_KEY or DEEPGRAM_API_KEY_FILE and retry")
		}
		return nil, fmt.Errorf("deepgram: %s: HTTP %d", route, resp.StatusCode)
	}
	return resp, nil
}

// ListVoices fetches current Aura models and their preview metadata.
func (c *Client) ListVoices(ctx context.Context) ([]tts.Voice, error) {
	return c.SearchVoices(ctx, "", 0)
}

// SearchVoices filters the current public Aura catalog by name or model ID.
func (c *Client) SearchVoices(ctx context.Context, search string, limit int) ([]tts.Voice, error) {
	resp, err := c.request(ctx, http.MethodGet, "/models", nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, errors.New("deepgram: voice catalog exceeds 4 MiB")
	}
	// Decode a named wire type so the public voice type stays provider-neutral.
	var wire struct {
		TTS []struct {
			Name          string   `json:"name"`
			CanonicalName string   `json:"canonical_name"`
			Architecture  string   `json:"architecture"`
			Languages     []string `json:"languages"`
			Metadata      struct {
				DisplayName string   `json:"display_name"`
				Accent      string   `json:"accent"`
				Sample      string   `json:"sample"`
				Tags        []string `json:"tags"`
			} `json:"metadata"`
		} `json:"tts"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	var voices []tts.Voice
	search = strings.ToLower(search)
	for _, model := range wire.TTS {
		// Flux uses a different endpoint and is not part of the Aura contract.
		if !strings.HasPrefix(model.CanonicalName, "aura-") {
			continue
		}
		name := model.Metadata.DisplayName
		if name == "" {
			name = model.Name
		}
		if !strings.Contains(strings.ToLower(name+" "+model.CanonicalName), search) {
			continue
		}
		voices = append(voices, tts.Voice{VoiceID: model.CanonicalName, Name: name, Category: model.Architecture,
			Description: strings.Join(model.Metadata.Tags, ", "), PreviewURL: model.Metadata.Sample,
			Labels: map[string]string{"language": strings.Join(model.Languages, ","), "accent": model.Metadata.Accent}})
	}
	sort.Slice(voices, func(i, j int) bool { return voices[i].VoiceID < voices[j].VoiceID })
	if limit > 0 && len(voices) > limit {
		voices = voices[:limit]
	}
	return voices, nil
}

// GetVoice resolves one Aura model, including its preview URL.
func (c *Client) GetVoice(ctx context.Context, voiceID string) (tts.Voice, error) {
	voices, err := c.ListVoices(ctx)
	if err != nil {
		return tts.Voice{}, err
	}
	for _, voice := range voices {
		if voice.VoiceID == voiceID {
			return voice, nil
		}
	}
	return tts.Voice{}, fmt.Errorf("unknown Deepgram voice %q; run sag voices --provider deepgram", voiceID)
}

// Synthesize emits complete PCM16 samples as they arrive and returns a WAV only
// after a complete HTTP response. The callback runs synchronously.
func (c *Client) Synthesize(ctx context.Context, text, voice string, speed float64, onPCM func([]byte) error) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("deepgram: input text is empty")
	}
	if !(speed >= 0.7 && speed <= 1.5) {
		return nil, errors.New("deepgram: speed must be between 0.7 and 1.5")
	}
	if voice == "" {
		voice = DefaultVoice
	}
	data, err := json.Marshal(struct {
		Text string `json:"text"`
	}{text})
	if err != nil {
		return nil, err
	}
	resp, err := c.request(ctx, http.MethodPost, "/speak", url.Values{
		"model": {voice}, "encoding": {"linear16"}, "sample_rate": {"24000"}, "container": {"none"},
		"speed": {strconv.FormatFloat(speed, 'f', -1, 64)},
	}, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "audio/l16" || params["rate"] != "24000" {
		return nil, errors.New("deepgram: returned an unexpected audio format; expected 24 kHz PCM16")
	}
	wav := make([]byte, 44)
	streamed := 44
	buffer := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if len(wav)-44+n > maxAudioBytes {
				return nil, errors.New("deepgram: audio exceeds 64 MiB")
			}
			wav = append(wav, buffer[:n]...)
			// HTTP chunks may split a sample; hold the final byte until its pair arrives.
			end := len(wav) &^ 1
			if onPCM != nil && end > streamed {
				if err := onPCM(wav[streamed:end]); err != nil {
					return nil, err
				}
			}
			streamed = end
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("deepgram: audio incomplete: %w", readErr)
		}
	}
	return tts.FinalizePCM16WAV(wav, 24000)
}
