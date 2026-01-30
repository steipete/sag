package minimax

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.minimax.io"

// Client talks to the MiniMax TTS API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient returns a client configured with the given API key and base URL.
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Voice represents a MiniMax voice entry.
type Voice struct {
	VoiceID     string
	Name        string
	Category    string
	Description string
}

type voiceEntry struct {
	VoiceID     string   `json:"voice_id"`
	VoiceName   string   `json:"voice_name"`
	Description []string `json:"description,omitempty"`
}

type listVoicesRequest struct {
	VoiceType string `json:"voice_type"`
}

type listVoicesResponse struct {
	SystemVoice     []voiceEntry `json:"system_voice"`
	VoiceCloning    []voiceEntry `json:"voice_cloning"`
	VoiceGeneration []voiceEntry `json:"voice_generation"`
	BaseResp        *baseResp    `json:"base_resp,omitempty"`
}

// ListVoices fetches available voices.
func (c *Client) ListVoices(ctx context.Context) ([]Voice, error) {
	u, err := c.httpURL("/v1/get_voice")
	if err != nil {
		return nil, err
	}

	reqBody, err := json.Marshal(listVoicesRequest{VoiceType: "all"})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list voices failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload listVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if err := payload.BaseResp.err(); err != nil {
		return nil, err
	}

	voices := make([]Voice, 0, len(payload.SystemVoice)+len(payload.VoiceCloning)+len(payload.VoiceGeneration))
	appendVoices := func(category string, entries []voiceEntry) {
		for _, v := range entries {
			name := strings.TrimSpace(v.VoiceName)
			if name == "" {
				name = v.VoiceID
			}
			voices = append(voices, Voice{
				VoiceID:     v.VoiceID,
				Name:        name,
				Category:    category,
				Description: strings.Join(v.Description, " "),
			})
		}
	}
	appendVoices("system", payload.SystemVoice)
	appendVoices("voice_cloning", payload.VoiceCloning)
	appendVoices("voice_generation", payload.VoiceGeneration)
	return voices, nil
}

// TTSRequest configures a text-to-speech request payload.
type TTSRequest struct {
	Model             string
	Text              string
	Speed             *float64
	Volume            *float64
	Pitch             *int
	Emotion           string
	TextNormalization *bool
	LatexRead         *bool
	AudioFormat       string
	SampleRate        int
	Bitrate           int
	Channel           int
	LanguageBoost     string
	ContinuousSound   *bool
	PronunciationDict *PronunciationDict
	VoiceModify       *VoiceModify
}

type baseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

func (b *baseResp) err() error {
	if b == nil {
		return nil
	}
	if b.StatusCode == 0 {
		return nil
	}
	msg := strings.TrimSpace(b.StatusMsg)
	if msg == "" {
		msg = "unknown error"
	}
	return fmt.Errorf("minimax error: %s (code=%d)", msg, b.StatusCode)
}

type voiceSetting struct {
	VoiceID           string   `json:"voice_id"`
	Speed             *float64 `json:"speed,omitempty"`
	Vol               *float64 `json:"vol,omitempty"`
	Pitch             *int     `json:"pitch,omitempty"`
	Emotion           string   `json:"emotion,omitempty"`
	TextNormalization *bool    `json:"text_normalization,omitempty"`
	LatexRead         *bool    `json:"latex_read,omitempty"`
}

type audioSetting struct {
	Format     string `json:"format,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Channel    int    `json:"channel,omitempty"`
}

// PronunciationDict configures pronunciation overrides.
type PronunciationDict struct {
	Tone []string `json:"tone,omitempty"`
}

// VoiceModify configures voice effects.
type VoiceModify struct {
	Pitch        *int    `json:"pitch,omitempty"`
	Intensity    *int    `json:"intensity,omitempty"`
	Timbre       *int    `json:"timbre,omitempty"`
	SoundEffects *string `json:"sound_effects,omitempty"`
}

type t2aRequest struct {
	Model             string             `json:"model"`
	Text              string             `json:"text"`
	Stream            bool               `json:"stream"`
	StreamOptions     *t2aStreamOptions  `json:"stream_options,omitempty"`
	OutputFormat      string             `json:"output_format,omitempty"`
	VoiceSetting      voiceSetting       `json:"voice_setting"`
	AudioSetting      audioSetting       `json:"audio_setting,omitempty"`
	LanguageBoost     string             `json:"language_boost,omitempty"`
	ContinuousSound   *bool              `json:"continuous_sound,omitempty"`
	PronunciationDict *PronunciationDict `json:"pronunciation_dict,omitempty"`
	VoiceModify       *VoiceModify       `json:"voice_modify,omitempty"`
}

type t2aStreamOptions struct {
	ExcludeAggregatedAudio bool `json:"exclude_aggregated_audio,omitempty"`
}

type t2aResponse struct {
	Data struct {
		Audio string `json:"audio"`
	} `json:"data"`
	BaseResp *baseResp `json:"base_resp,omitempty"`
}

type t2aStreamData struct {
	Audio  string `json:"audio,omitempty"`
	Status int    `json:"status,omitempty"`
}

type t2aStreamResponse struct {
	Data     *t2aStreamData `json:"data,omitempty"`
	BaseResp *baseResp      `json:"base_resp,omitempty"`
}

// ConvertTTS downloads the full audio before returning.
func (c *Client) ConvertTTS(ctx context.Context, voiceID string, req TTSRequest) ([]byte, error) {
	u, err := c.httpURL("/v1/t2a_v2")
	if err != nil {
		return nil, err
	}

	payload := t2aRequest{
		Model:             req.Model,
		Text:              req.Text,
		Stream:            false,
		OutputFormat:      "hex",
		VoiceSetting:      buildVoiceSetting(voiceID, req),
		AudioSetting:      buildAudioSetting(req),
		LanguageBoost:     req.LanguageBoost,
		ContinuousSound:   req.ContinuousSound,
		PronunciationDict: req.PronunciationDict,
		VoiceModify:       req.VoiceModify,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("convert TTS failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var response t2aResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if err := response.BaseResp.err(); err != nil {
		return nil, err
	}
	if response.Data.Audio == "" {
		return nil, errors.New("minimax response missing audio")
	}

	data, err := hex.DecodeString(response.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("decode audio hex: %w", err)
	}
	return data, nil
}

type cancelReadCloser struct {
	*io.PipeReader
	cancel func()
}

func (c *cancelReadCloser) Close() error {
	c.cancel()
	return c.PipeReader.Close()
}

// StreamTTS streams MP3 audio from MiniMax via HTTP (SSE).
func (c *Client) StreamTTS(ctx context.Context, voiceID string, req TTSRequest) (io.ReadCloser, error) {
	u, err := c.httpURL("/v1/t2a_v2")
	if err != nil {
		return nil, err
	}

	payload := t2aRequest{
		Model:             req.Model,
		Text:              req.Text,
		Stream:            true,
		StreamOptions:     &t2aStreamOptions{ExcludeAggregatedAudio: true},
		OutputFormat:      "hex",
		VoiceSetting:      buildVoiceSetting(voiceID, req),
		AudioSetting:      buildAudioSetting(req),
		LanguageBoost:     req.LanguageBoost,
		ContinuousSound:   req.ContinuousSound,
		PronunciationDict: req.PronunciationDict,
		VoiceModify:       req.VoiceModify,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		cancel()
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("stream TTS failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	pr, pw := io.Pipe()
	go func() {
		defer cancel()
		defer func() { _ = resp.Body.Close() }()
		if err := readMiniMaxStream(ctx, resp.Body, pw); err != nil && !errors.Is(err, context.Canceled) {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	return &cancelReadCloser{PipeReader: pr, cancel: cancel}, nil
}

func readMiniMaxStream(ctx context.Context, body io.Reader, pw *io.PipeWriter) error {
	reader := bufio.NewReader(body)
	var dataLines []string
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF && len(line) == 0 {
			break
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) > 0 {
				done, err := handleMiniMaxStreamPayload(strings.Join(dataLines, "\n"), pw)
				if err != nil {
					return err
				}
				if done {
					return nil
				}
				dataLines = dataLines[:0]
			}
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		} else if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:") {
			// Ignore SSE metadata/comments.
		} else {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				done, err := handleMiniMaxStreamPayload(trimmed, pw)
				if err != nil {
					return err
				}
				if done {
					return nil
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	if len(dataLines) > 0 {
		done, err := handleMiniMaxStreamPayload(strings.Join(dataLines, "\n"), pw)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return nil
}

func handleMiniMaxStreamPayload(payload string, pw *io.PipeWriter) (bool, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return false, nil
	}

	var items []t2aStreamResponse
	if strings.HasPrefix(payload, "[") {
		if err := json.Unmarshal([]byte(payload), &items); err != nil {
			return false, err
		}
	} else {
		var item t2aStreamResponse
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
			return false, err
		}
		items = append(items, item)
	}

	for _, item := range items {
		if err := item.BaseResp.err(); err != nil {
			return false, err
		}
		if item.Data != nil && item.Data.Audio != "" {
			chunk, err := hex.DecodeString(item.Data.Audio)
			if err != nil {
				return false, fmt.Errorf("decode audio chunk: %w", err)
			}
			if len(chunk) > 0 {
				if _, err := pw.Write(chunk); err != nil {
					return false, err
				}
			}
		}
		if item.Data != nil && item.Data.Status == 2 {
			return true, nil
		}
	}
	return false, nil
}

func buildVoiceSetting(voiceID string, req TTSRequest) voiceSetting {
	return voiceSetting{
		VoiceID:           voiceID,
		Speed:             req.Speed,
		Vol:               req.Volume,
		Pitch:             req.Pitch,
		Emotion:           req.Emotion,
		TextNormalization: req.TextNormalization,
		LatexRead:         req.LatexRead,
	}
}

func buildAudioSetting(req TTSRequest) audioSetting {
	return audioSetting{
		Format:     req.AudioFormat,
		SampleRate: req.SampleRate,
		Bitrate:    req.Bitrate,
		Channel:    req.Channel,
	}
}

func (c *Client) httpURL(endpoint string) (string, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, endpoint)
	return u.String(), nil
}
