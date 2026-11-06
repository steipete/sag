// Package sixtydb provides a strict adapter for the live 60db HTTP TTS API.
package sixtydb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/steipete/sag/internal/tts"
)

// DefaultBaseURL is the public 60db API host.
const DefaultBaseURL = "https://api.60db.ai"

const (
	// DefaultSampleRate matches the live 60db PCM response.
	DefaultSampleRate    = 48000
	maxDecodedAudioBytes = 96 << 20
	maxDecodedChunkBytes = 8 << 20
	maxStreamFrameBytes  = 12 << 20
)

type audioFormat string

const (
	audioFormatMP3  audioFormat = "mp3"
	audioFormatWAV  audioFormat = "wav"
	audioFormatOGG  audioFormat = "ogg"
	audioFormatFLAC audioFormat = "flac"
)

// Client talks to the 60db HTTP API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// TTSRequest is the 60db TTS payload shape exposed by the CLI.
type TTSRequest struct {
	Text         string   `json:"text"`
	VoiceID      string   `json:"voice_id,omitempty"`
	Speed        *float64 `json:"speed,omitempty"`
	Stability    *float64 `json:"stability,omitempty"`
	Similarity   *float64 `json:"similarity,omitempty"`
	OutputFormat string   `json:"output_format,omitempty"`
	SampleRate   int      `json:"sample_rate,omitempty"`
}

// NewClient returns a Client configured with the given API key and base URL.
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

func (c *Client) newRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, endpoint)

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	return req, nil
}

type envelope struct {
	Success *bool  `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (c *Client) errorFromEnvelope(op string, env envelope, fallback string) error {
	msg := strings.TrimSpace(env.Message)
	if msg == "" {
		msg = strings.TrimSpace(env.Error)
	}
	if msg == "" {
		msg = fallback
	}
	return fmt.Errorf("%s: %s", op, c.sanitize(msg))
}

func (c *Client) httpError(op string, status string, body []byte) error {
	var env envelope
	if len(body) > 0 && json.Unmarshal(body, &env) == nil {
		return c.errorFromEnvelope(op, env, status)
	}
	return fmt.Errorf("%s: %s", op, status)
}

func (c *Client) sanitize(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" || c.apiKey == "" {
		return msg
	}
	msg = strings.ReplaceAll(msg, "Bearer "+c.apiKey, "Bearer [redacted]")
	msg = strings.ReplaceAll(msg, c.apiKey, "[redacted]")
	return msg
}

// CanonicalOutputFormat maps CLI/ElevenLabs-style output names to the simple
// 60db formats documented for /tts-synthesize.
func CanonicalOutputFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch {
	case format == "":
		return ""
	case strings.HasPrefix(format, "mp3"):
		return string(audioFormatMP3)
	case strings.HasPrefix(format, "pcm"), strings.HasPrefix(format, "wav"):
		return string(audioFormatWAV)
	case strings.HasPrefix(format, "opus"), strings.HasPrefix(format, "ogg"):
		return string(audioFormatOGG)
	case strings.HasPrefix(format, "flac"):
		return string(audioFormatFLAC)
	default:
		return format
	}
}

func parseAudioFormat(value string) audioFormat {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(audioFormatMP3):
		return audioFormatMP3
	case string(audioFormatWAV):
		return audioFormatWAV
	case string(audioFormatOGG):
		return audioFormatOGG
	case string(audioFormatFLAC):
		return audioFormatFLAC
	default:
		return ""
	}
}

func sniffAudioFormat(data []byte) (audioFormat, error) {
	if len(data) == 0 {
		return "", errors.New("empty audio")
	}
	switch {
	case len(data) >= 3 && string(data[:3]) == "ID3":
		return audioFormatMP3, nil
	case len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0:
		return audioFormatMP3, nil
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return audioFormatWAV, nil
	case len(data) >= 4 && string(data[:4]) == "OggS":
		return audioFormatOGG, nil
	case len(data) >= 4 && string(data[:4]) == "fLaC":
		return audioFormatFLAC, nil
	default:
		return "", errors.New("unrecognized audio format")
	}
}

type voiceEntry struct {
	VoiceID     string            `json:"voice_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Model       string            `json:"model"`
	Labels      map[string]string `json:"labels"`
	Description *string           `json:"description"`
	Categories  []string          `json:"categories"`
}

type voicesResponse struct {
	envelope
	Data []voiceEntry `json:"data"`
}

type voiceDetailsResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Gender      string   `json:"gender"`
	Age         string   `json:"age"`
	Accent      string   `json:"accent"`
	UseCase     []string `json:"use_case"`
	SampleURL   string   `json:"sample_url"`
	IsCustom    bool     `json:"is_custom"`
}

func (e voiceEntry) toVoice(source string) tts.Voice {
	labels := make(map[string]string, len(e.Labels)+2)
	for k, v := range e.Labels {
		labels[k] = v
	}
	if e.Model != "" {
		labels["model"] = e.Model
	}
	if len(e.Categories) > 0 {
		labels["categories"] = strings.Join(e.Categories, ", ")
	}
	if source != "" {
		labels["source"] = source
	}
	description := ""
	if e.Description != nil {
		description = strings.TrimSpace(*e.Description)
	}
	return tts.Voice{
		VoiceID:     e.VoiceID,
		Name:        e.Name,
		Category:    e.Category,
		Description: description,
		Labels:      labels,
	}
}

func (v voiceDetailsResponse) toVoice() tts.Voice {
	labels := map[string]string{}
	if v.Language != "" {
		labels["language"] = v.Language
	}
	if v.Gender != "" {
		labels["gender"] = v.Gender
	}
	if v.Age != "" {
		labels["age"] = v.Age
	}
	if v.Accent != "" {
		labels["accent"] = v.Accent
	}
	if len(v.UseCase) > 0 {
		labels["use_case"] = strings.Join(v.UseCase, ", ")
	}
	if v.IsCustom {
		labels["source"] = "myvoices"
	}
	return tts.Voice{
		VoiceID:     v.ID,
		Name:        v.Name,
		Description: strings.TrimSpace(v.Description),
		Labels:      labels,
		PreviewURL:  strings.TrimSpace(v.SampleURL),
	}
}

func (c *Client) fetchVoices(ctx context.Context, endpoint, source string) ([]tts.Voice, error) {
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, c.httpError("list voices", resp.Status, body)
	}

	var body voicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("list voices: decode response: %w", err)
	}
	if body.Success == nil || !*body.Success {
		return nil, c.errorFromEnvelope("list voices", body.envelope, "unexpected response envelope")
	}

	voices := make([]tts.Voice, 0, len(body.Data))
	for _, entry := range body.Data {
		voices = append(voices, entry.toVoice(source))
	}
	return voices, nil
}

// ListVoices merges the documented default and user-created voice catalogs.
func (c *Client) ListVoices(ctx context.Context) ([]tts.Voice, error) {
	defaultVoices, err := c.fetchVoices(ctx, "/default-voices", "default")
	if err != nil {
		return nil, err
	}
	myVoices, err := c.fetchVoices(ctx, "/myvoices", "myvoices")
	if err != nil {
		return nil, err
	}

	merged := make([]tts.Voice, 0, len(defaultVoices)+len(myVoices))
	seen := make(map[string]struct{}, len(defaultVoices)+len(myVoices))
	for _, voice := range append(defaultVoices, myVoices...) {
		if voice.VoiceID == "" {
			continue
		}
		if _, ok := seen[voice.VoiceID]; ok {
			continue
		}
		seen[voice.VoiceID] = struct{}{}
		merged = append(merged, voice)
	}
	return merged, nil
}

// SearchVoices filters the merged voice catalog by name.
func (c *Client) SearchVoices(ctx context.Context, search string, limit int) ([]tts.Voice, error) {
	voices, err := c.ListVoices(ctx)
	if err != nil {
		return nil, err
	}
	search = strings.TrimSpace(search)
	if search != "" {
		searchLower := strings.ToLower(search)
		filtered := make([]tts.Voice, 0, len(voices))
		for _, voice := range voices {
			if strings.Contains(strings.ToLower(voice.Name), searchLower) {
				filtered = append(filtered, voice)
			}
		}
		voices = filtered
	}
	if limit > 0 && len(voices) > limit {
		voices = voices[:limit]
	}
	return voices, nil
}

// GetVoice resolves a voice from the documented per-voice endpoint.
func (c *Client) GetVoice(ctx context.Context, voiceID string) (tts.Voice, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path.Join("/voices", voiceID), nil)
	if err != nil {
		return tts.Voice{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return tts.Voice{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return tts.Voice{}, c.httpError("get voice", resp.Status, body)
	}

	var body voiceDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return tts.Voice{}, fmt.Errorf("get voice: decode response: %w", err)
	}
	if strings.TrimSpace(body.ID) == "" {
		return tts.Voice{}, errors.New("get voice: missing voice id")
	}
	return body.toVoice(), nil
}

type synthResponse struct {
	envelope
	AudioBase64     string  `json:"audio_base64"`
	SampleRate      int     `json:"sample_rate"`
	DurationSeconds float64 `json:"duration_seconds"`
	Encoding        string  `json:"encoding"`
	OutputFormat    string  `json:"output_format"`
}

type synthEnvelope struct {
	synthResponse
	BackendResponse *synthResponse `json:"backendResponse"`
}

func validateRequestedFormat(requested string) error {
	if requested == "" {
		return nil
	}
	if parseAudioFormat(requested) == "" {
		return fmt.Errorf("unsupported 60db output format %q", requested)
	}
	return nil
}

func validateConvertResponseFormat(requested string, resp synthResponse, data []byte) error {
	sniffed, err := sniffAudioFormat(data)
	if err != nil {
		return err
	}

	declared := parseAudioFormat(resp.OutputFormat)
	if declared == "" {
		declared = parseAudioFormat(resp.Encoding)
	}
	if declared != "" && declared != sniffed {
		return fmt.Errorf("response format mismatch: declared %s, decoded %s", declared, sniffed)
	}

	expected := parseAudioFormat(requested)
	if expected != "" && sniffed != expected {
		return fmt.Errorf("response format mismatch: requested %s, decoded %s", expected, sniffed)
	}
	return nil
}

func validateAudioFormat(requested string, data []byte) error {
	sniffed, err := sniffAudioFormat(data)
	if err != nil {
		return err
	}
	expected := parseAudioFormat(requested)
	if expected != "" && sniffed != expected {
		return fmt.Errorf("response format mismatch: requested %s, decoded %s", expected, sniffed)
	}
	return nil
}

func wrapPCM16LEWAV(pcm []byte, sampleRate int) ([]byte, error) {
	if len(pcm) == 0 {
		return nil, errors.New("empty PCM audio")
	}
	if len(pcm)%2 != 0 {
		return nil, errors.New("PCM audio has an incomplete 16-bit sample")
	}
	if sampleRate <= 0 {
		return nil, errors.New("invalid PCM sample rate")
	}

	const (
		headerSize    = 44
		channelCount  = 1
		bitsPerSample = 16
	)
	if uint64(len(pcm)) > uint64(^uint32(0))-headerSize {
		return nil, errors.New("PCM audio is too large for WAV")
	}

	dataSize := uint32(len(pcm))
	byteRate := uint32(sampleRate * channelCount * bitsPerSample / 8)
	blockAlign := uint16(channelCount * bitsPerSample / 8)
	wav := make([]byte, headerSize+len(pcm))
	copy(wav[0:4], "RIFF")
	putUint32LE(wav[4:8], 36+dataSize)
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	putUint32LE(wav[16:20], 16)
	putUint16LE(wav[20:22], 1)
	putUint16LE(wav[22:24], channelCount)
	putUint32LE(wav[24:28], uint32(sampleRate))
	putUint32LE(wav[28:32], byteRate)
	putUint16LE(wav[32:34], blockAlign)
	putUint16LE(wav[34:36], bitsPerSample)
	copy(wav[36:40], "data")
	putUint32LE(wav[40:44], dataSize)
	copy(wav[44:], pcm)
	return wav, nil
}

func putUint16LE(dst []byte, value uint16) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
}

func putUint32LE(dst []byte, value uint32) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
	dst[2] = byte(value >> 16)
	dst[3] = byte(value >> 24)
}

func decodeBase64Audio(encoded string, limit int, label string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("%s is empty", label)
	}
	decodedLen := base64.StdEncoding.DecodedLen(len(encoded))
	if decodedLen <= 0 {
		return nil, fmt.Errorf("%s decoded to empty audio", label)
	}
	if decodedLen > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}

	decoded := make([]byte, decodedLen)
	n, err := base64.StdEncoding.Decode(decoded, []byte(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	decoded = decoded[:n]
	if len(decoded) == 0 {
		return nil, fmt.Errorf("%s decoded to empty audio", label)
	}
	return decoded, nil
}

func decodeAudioContent(encoded string, totalBytes int64) ([]byte, error) {
	decoded, err := decodeBase64Audio(encoded, maxStreamFrameBytes, "audio chunk")
	if err != nil {
		return nil, err
	}

	if bytes.HasPrefix(bytes.TrimSpace(decoded), []byte("{")) {
		var inner struct {
			Result struct {
				AudioContent string `json:"audioContent"`
			} `json:"result"`
			AudioContent string `json:"audioContent"`
		}
		if err := json.Unmarshal(decoded, &inner); err == nil {
			encoded = inner.Result.AudioContent
			if encoded == "" {
				encoded = inner.AudioContent
			}
			if encoded != "" {
				decoded, err = decodeBase64Audio(encoded, maxDecodedChunkBytes, "nested audio chunk")
				if err != nil {
					return nil, err
				}
			}
		}
	}
	if len(decoded) > maxDecodedChunkBytes {
		return nil, fmt.Errorf("audio chunk exceeds %d bytes", maxDecodedChunkBytes)
	}

	if totalBytes+int64(len(decoded)) > maxDecodedAudioBytes {
		return nil, fmt.Errorf("audio exceeds %d bytes", maxDecodedAudioBytes)
	}
	return decoded, nil
}

type liveSynthFrame struct {
	envelope
	Type   string `json:"type"`
	Result struct {
		AudioContent string `json:"audioContent"`
	} `json:"result"`
	AudioContent string   `json:"audioContent"`
	Incomplete   bool     `json:"incomplete"`
	Reasons      []string `json:"reasons"`
}

func (c *Client) decodeNDJSONAudio(ctx context.Context, src io.Reader, requested string, sampleRate int) ([]byte, error) {
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 64*1024), maxStreamFrameBytes)

	var audio bytes.Buffer
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var frame liveSynthFrame
		if err := json.Unmarshal(line, &frame); err != nil {
			return nil, fmt.Errorf("synthesize audio: decode NDJSON frame: %w", err)
		}
		if frame.Success != nil && !*frame.Success {
			return nil, c.errorFromEnvelope("synthesize audio", frame.envelope, "request failed")
		}
		if frame.Incomplete {
			reason := strings.Join(frame.Reasons, ", ")
			if reason == "" {
				reason = "provider returned incomplete audio"
			}
			return nil, fmt.Errorf("synthesize audio: incomplete response: %s", c.sanitize(reason))
		}
		if frame.Type == "error" {
			return nil, c.errorFromEnvelope("synthesize audio", frame.envelope, "provider returned an error frame")
		}

		encoded := frame.Result.AudioContent
		if encoded == "" {
			encoded = frame.AudioContent
		}
		if encoded == "" {
			continue
		}
		chunk, err := decodeAudioContent(encoded, int64(audio.Len()))
		if err != nil {
			return nil, fmt.Errorf("synthesize audio: %w", err)
		}
		_, _ = audio.Write(chunk)
	}
	if err := scanner.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("synthesize audio: read NDJSON frame: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if audio.Len() == 0 {
		return nil, errors.New("synthesize audio: response contained no audio chunks")
	}

	data := audio.Bytes()
	if _, err := sniffAudioFormat(data); err == nil {
		if err := validateAudioFormat(requested, data); err != nil {
			return nil, fmt.Errorf("synthesize audio: %w", err)
		}
		return append([]byte(nil), data...), nil
	}
	if expected := parseAudioFormat(requested); expected != "" && expected != audioFormatWAV {
		return nil, fmt.Errorf("synthesize audio: provider returned raw PCM for requested %s output", expected)
	}
	wav, err := wrapPCM16LEWAV(data, sampleRate)
	if err != nil {
		return nil, fmt.Errorf("synthesize audio: %w", err)
	}
	return wav, nil
}

func readLimitedAudio(src io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(src, maxDecodedAudioBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty audio")
	}
	if len(data) > maxDecodedAudioBytes {
		return nil, fmt.Errorf("audio exceeds %d bytes", maxDecodedAudioBytes)
	}
	return data, nil
}

// ConvertTTS downloads the full audio and returns decoded bytes.
func (c *Client) ConvertTTS(ctx context.Context, reqBody TTSRequest) ([]byte, error) {
	reqBody.OutputFormat = CanonicalOutputFormat(reqBody.OutputFormat)
	if err := validateRequestedFormat(reqBody.OutputFormat); err != nil {
		return nil, err
	}
	if reqBody.SampleRate == 0 {
		reqBody.SampleRate = DefaultSampleRate
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/tts-synthesize", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson, application/json, audio/*, application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, c.httpError("synthesize audio", resp.Status, body)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/x-ndjson") || strings.Contains(contentType, "ndjson") {
		return c.decodeNDJSONAudio(ctx, resp.Body, reqBody.OutputFormat, reqBody.SampleRate)
	}
	if strings.Contains(contentType, "audio/") || strings.Contains(contentType, "octet-stream") {
		data, err := readLimitedAudio(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("synthesize audio: %w", err)
		}
		if _, err := sniffAudioFormat(data); err == nil {
			if err := validateAudioFormat(reqBody.OutputFormat, data); err != nil {
				return nil, fmt.Errorf("synthesize audio: %w", err)
			}
			return data, nil
		}
		if expected := parseAudioFormat(reqBody.OutputFormat); expected != "" && expected != audioFormatWAV {
			return nil, fmt.Errorf("synthesize audio: provider returned raw PCM for requested %s output", expected)
		}
		wav, err := wrapPCM16LEWAV(data, reqBody.SampleRate)
		if err != nil {
			return nil, fmt.Errorf("synthesize audio: %w", err)
		}
		return wav, nil
	}

	var response synthEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("synthesize audio: decode response: %w", err)
	}
	body := response.synthResponse
	if response.BackendResponse != nil {
		body = *response.BackendResponse
		if body.Success == nil {
			body.Success = response.Success
		}
	}
	if body.Success == nil || !*body.Success {
		return nil, c.errorFromEnvelope("synthesize audio", body.envelope, "unexpected response envelope")
	}
	if strings.TrimSpace(body.AudioBase64) == "" {
		return nil, errors.New("synthesize audio: empty audio_base64")
	}

	data, err := decodeBase64Audio(body.AudioBase64, maxDecodedAudioBytes, "audio_base64")
	if err != nil {
		return nil, fmt.Errorf("synthesize audio: %w", err)
	}
	if err := validateConvertResponseFormat(reqBody.OutputFormat, body, data); err != nil {
		return nil, fmt.Errorf("synthesize audio: %w", err)
	}
	return data, nil
}

// StreamTTS preserves the streaming client shape while using the live
// /tts-synthesize contract, which must be fully validated before playback.
func (c *Client) StreamTTS(ctx context.Context, reqBody TTSRequest) (io.ReadCloser, error) {
	data, err := c.ConvertTTS(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
