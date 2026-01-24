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

// Client talks to the MiniMax HTTP API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient returns a Client configured with the given API key and base URL.
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	hc := defaultHTTPClient
	if hc == nil {
		hc = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: hc,
	}
}

// defaultHTTPClient allows tests to override the HTTP transport.
var defaultHTTPClient *http.Client

// SetHTTPClient overrides the HTTP client used by new MiniMax clients.
func SetHTTPClient(c *http.Client) {
	defaultHTTPClient = c
}

// BaseResp contains MiniMax API status info.
type BaseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// Voice represents a voice entry returned by MiniMax.
type Voice struct {
	VoiceID     string   `json:"voice_id"`
	VoiceName   string   `json:"voice_name,omitempty"`
	Description []string `json:"description,omitempty"`
	CreatedTime string   `json:"created_time,omitempty"`
	Category    string   `json:"-"`
}

type getVoiceRequest struct {
	VoiceType string `json:"voice_type"`
}

type getVoiceResponse struct {
	SystemVoice     []Voice   `json:"system_voice"`
	VoiceCloning    []Voice   `json:"voice_cloning"`
	VoiceGeneration []Voice   `json:"voice_generation"`
	BaseResp        *BaseResp `json:"base_resp,omitempty"`
}

// ListVoices fetches available voices. voiceType should be one of:
// system, voice_cloning, voice_generation, all.
func (c *Client) ListVoices(ctx context.Context, voiceType string) ([]Voice, error) {
	if voiceType == "" {
		voiceType = "all"
	}
	reqBody := getVoiceRequest{VoiceType: voiceType}
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/get_voice", reqBody, "application/json")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list voices failed: %s", resp.Status)
	}

	var body getVoiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if err := baseRespError(body.BaseResp); err != nil {
		return nil, err
	}

	voices := make([]Voice, 0, len(body.SystemVoice)+len(body.VoiceCloning)+len(body.VoiceGeneration))
	appendCategory := func(list []Voice, category string) {
		for _, v := range list {
			v.Category = category
			voices = append(voices, v)
		}
	}
	appendCategory(body.SystemVoice, "system")
	appendCategory(body.VoiceCloning, "voice_cloning")
	appendCategory(body.VoiceGeneration, "voice_generation")
	return voices, nil
}

// TTSRequest configures a text-to-audio request payload.
type TTSRequest struct {
	Model           string         `json:"model"`
	Text            string         `json:"text"`
	Stream          bool           `json:"stream,omitempty"`
	StreamOptions   *StreamOptions `json:"stream_options,omitempty"`
	VoiceSetting    *VoiceSetting  `json:"voice_setting,omitempty"`
	AudioSetting    *AudioSetting  `json:"audio_setting,omitempty"`
	OutputFormat    string         `json:"output_format,omitempty"`
	LanguageBoost   string         `json:"language_boost,omitempty"`
	SubtitleEnable  *bool          `json:"subtitle_enable,omitempty"`
	ContinuousSound *bool          `json:"continuous_sound,omitempty"`
}

// StreamOptions tunes streaming output behavior.
type StreamOptions struct {
	ExcludeAggregatedAudio bool `json:"exclude_aggregated_audio,omitempty"`
}

// VoiceSetting controls synthesis parameters.
type VoiceSetting struct {
	VoiceID           string   `json:"voice_id"`
	Speed             *float64 `json:"speed,omitempty"`
	Vol               *float64 `json:"vol,omitempty"`
	Pitch             *int     `json:"pitch,omitempty"`
	Emotion           string   `json:"emotion,omitempty"`
	TextNormalization *bool    `json:"text_normalization,omitempty"`
}

// AudioSetting configures the generated audio format.
type AudioSetting struct {
	SampleRate *int   `json:"sample_rate,omitempty"`
	Bitrate    *int   `json:"bitrate,omitempty"`
	Format     string `json:"format,omitempty"`
	Channel    *int   `json:"channel,omitempty"`
	ForceCBR   *bool  `json:"force_cbr,omitempty"`
}

// TTSResponse contains audio data or stream chunks.
type TTSResponse struct {
	Data      *TTSResponseData `json:"data"`
	TraceID   string           `json:"trace_id"`
	ExtraInfo *TTSExtraInfo    `json:"extra_info,omitempty"`
	BaseResp  *BaseResp        `json:"base_resp"`
}

// TTSResponseData holds audio chunk data.
type TTSResponseData struct {
	Audio        string `json:"audio"`
	SubtitleFile string `json:"subtitle_file,omitempty"`
	Status       int    `json:"status"`
}

// TTSExtraInfo contains optional metadata about synthesized audio.
type TTSExtraInfo struct {
	AudioLength             int     `json:"audio_length,omitempty"`
	AudioSampleRate         int     `json:"audio_sample_rate,omitempty"`
	AudioSize               int     `json:"audio_size,omitempty"`
	Bitrate                 int     `json:"bitrate,omitempty"`
	AudioFormat             string  `json:"audio_format,omitempty"`
	AudioChannel            int     `json:"audio_channel,omitempty"`
	InvisibleCharacterRatio float64 `json:"invisible_character_ratio,omitempty"`
	UsageCharacters         int     `json:"usage_characters,omitempty"`
	WordCount               int     `json:"word_count,omitempty"`
}

// StreamTTS requests streaming audio for text-to-audio.
func (c *Client) StreamTTS(ctx context.Context, payload TTSRequest) (io.ReadCloser, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/t2a_v2", payload, "text/event-stream")
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

	pr, pw := io.Pipe()
	stream := &ttsStream{PipeReader: pr, respBody: resp.Body}
	contentType := resp.Header.Get("Content-Type")
	go func() {
		defer func() {
			_ = resp.Body.Close()
		}()
		if strings.Contains(contentType, "text/event-stream") {
			pipeErr := streamFromSSE(resp.Body, pw)
			_ = pw.CloseWithError(pipeErr)
			return
		}
		pipeErr := streamFromJSON(resp.Body, pw)
		_ = pw.CloseWithError(pipeErr)
	}()
	return stream, nil
}

// ConvertTTS downloads the full audio before returning.
func (c *Client) ConvertTTS(ctx context.Context, payload TTSRequest) ([]byte, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/t2a_v2", payload, "application/json")
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

	var body TTSResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if err := baseRespError(body.BaseResp); err != nil {
		return nil, err
	}
	if body.Data == nil || body.Data.Audio == "" {
		return nil, errors.New("TTS response missing audio data")
	}
	return decodeHexAudio(body.Data.Audio)
}

type ttsStream struct {
	*io.PipeReader
	respBody io.ReadCloser
}

func (s *ttsStream) Close() error {
	_ = s.respBody.Close()
	return s.PipeReader.Close()
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, payload any, accept string) (*http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, endpoint)

	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	return c.httpClient.Do(req)
}

func baseRespError(base *BaseResp) error {
	if base == nil {
		return nil
	}
	if base.StatusCode != 0 {
		return fmt.Errorf("minimax error %d: %s", base.StatusCode, base.StatusMsg)
	}
	return nil
}

func streamFromSSE(r io.Reader, w io.Writer) error {
	reader := bufio.NewReader(r)
	var dataLines []string
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		return handleStreamPayload(payload, w)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return flush()
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func streamFromJSON(r io.Reader, w io.Writer) error {
	payload, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil
	}

	var items []TTSResponse
	if err := json.Unmarshal(payload, &items); err == nil {
		for _, item := range items {
			if err := handleStreamResponse(item, w); err != nil {
				return err
			}
		}
		return nil
	}

	var item TTSResponse
	if err := json.Unmarshal(payload, &item); err != nil {
		return err
	}
	return handleStreamResponse(item, w)
}

func handleStreamPayload(payload string, w io.Writer) error {
	if payload == "" || payload == "[DONE]" {
		return nil
	}
	var item TTSResponse
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		return err
	}
	return handleStreamResponse(item, w)
}

func handleStreamResponse(item TTSResponse, w io.Writer) error {
	if err := baseRespError(item.BaseResp); err != nil {
		return err
	}
	if item.Data == nil || item.Data.Audio == "" {
		return nil
	}
	audio, err := decodeHexAudio(item.Data.Audio)
	if err != nil {
		return err
	}
	_, err = w.Write(audio)
	return err
}

func decodeHexAudio(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode audio hex: %w", err)
	}
	return data, nil
}
