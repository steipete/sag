package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/steipete/sag/internal/minimax"
)

func TestInferFormatFromExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"out.mp3", "mp3_44100_128"},
		{"out.MP3", "mp3_44100_128"},
		{"audio.wav", "wav"},
		{"audio.WAVE", "wav"},
		{"audio.flac", "flac"},
		{"audio.unknown", ""},
	}
	for _, tt := range tests {
		if got := inferFormatFromExt(tt.path); got != tt.want {
			t.Fatalf("inferFormatFromExt(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestResolveTextFromArgs(t *testing.T) {
	got, err := resolveText([]string{"hello", "world"}, "")
	if err != nil {
		t.Fatalf("resolveText args error: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("resolveText args = %q, want %q", got, "hello world")
	}
}

func TestResolveTextFromFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "sag_text")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString("from file"); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}

	got, err := resolveText(nil, tmp.Name())
	if err != nil {
		t.Fatalf("resolveText file error: %v", err)
	}
	if got != "from file" {
		t.Fatalf("resolveText file = %q, want %q", got, "from file")
	}
}

func TestResolveTextFromStdin(t *testing.T) {
	orig := os.Stdin
	defer func() { os.Stdin = orig }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString("from stdin"); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	os.Stdin = r

	got, err := resolveText(nil, "")
	if err != nil {
		t.Fatalf("resolveText stdin error: %v", err)
	}
	if got != "from stdin" {
		t.Fatalf("resolveText stdin = %q, want %q", got, "from stdin")
	}
}

func TestResolveTextFileNotFound(t *testing.T) {
	if _, err := resolveText(nil, "/tmp/does-not-exist-sag"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestResolveTextEmptySources(t *testing.T) {
	// With no args, no file, and stdin still a TTY, expect an error.
	if _, err := resolveText(nil, ""); err == nil {
		t.Fatalf("expected error when no text is provided")
	}
}

func TestResolveTextEmptyFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "sag_empty")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}

	if _, err := resolveText(nil, tmp.Name()); err == nil {
		t.Fatalf("expected error on empty input file")
	}
}

func TestApplyRateOverridesInvalidSpeed(t *testing.T) {
	opts := &speakOptions{speed: 0.3, rateWPM: 200}
	if err := applyRateAndSpeed(opts); err != nil {
		t.Fatalf("applyRateAndSpeed error: %v", err)
	}
	want := float64(200) / float64(defaultWPM)
	if math.Abs(opts.speed-want) > 1e-9 {
		t.Fatalf("expected speed %.2f, got %.2f", want, opts.speed)
	}
}

func TestApplyRateAndSpeedInvalidSpeed(t *testing.T) {
	opts := &speakOptions{speed: 0.3}
	if err := applyRateAndSpeed(opts); err == nil {
		t.Fatalf("expected speed validation error")
	}
}

func TestResolveVoiceDefaultsToFirst(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeVoiceList(w, []minimax.Voice{{VoiceID: "id1", VoiceName: "Alpha"}, {VoiceID: "id2", VoiceName: "Beta"}})
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	id, err := resolveVoice(context.Background(), client, "", "all", false)
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != "id1" {
		t.Fatalf("resolveVoice default id = %q, want id1", id)
	}
}

func TestResolveVoiceForceIDPassThrough(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatalf("server should not be called for forced ID pass-through")
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	input := "custom-voice-id"
	id, err := resolveVoice(context.Background(), client, input, "all", true)
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != input {
		t.Fatalf("expected ID to pass through, got %q", id)
	}
}

func TestResolveVoiceExactIDMatch(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeVoiceList(w, []minimax.Voice{{VoiceID: "voice-123", VoiceName: "Alpha"}})
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	id, err := resolveVoice(context.Background(), client, "voice-123", "all", false)
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != "voice-123" {
		t.Fatalf("expected voice-123, got %q", id)
	}
}

func TestResolveVoiceNameMatch(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeVoiceList(w, []minimax.Voice{{VoiceID: "id-sarah", VoiceName: "Sarah"}, {VoiceID: "id-roger", VoiceName: "Roger"}})
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	id, err := resolveVoice(context.Background(), client, "roger", "all", false)
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != "id-roger" {
		t.Fatalf("resolveVoice by name = %q, want id-roger", id)
	}
}

func TestResolveVoicePartialMatch(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeVoiceList(w, []minimax.Voice{{VoiceID: "id1", VoiceName: "Sarah"}, {VoiceID: "id2", VoiceName: "Roger - Casual"}})
	}))
	defer restoreHTTP()

	restore, read := captureStderr(t)
	defer restore()

	client := minimax.NewClient("key", "http://minimax.test")
	id, err := resolveVoice(context.Background(), client, "roger", "all", false)
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != "id2" {
		t.Fatalf("expected id2 for partial match 'roger', got %q", id)
	}
	if out := read(); !strings.Contains(out, "using voice") {
		t.Fatalf("expected 'using voice' notice, got %q", out)
	}
}

func TestResolveVoiceNoMatch(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeVoiceList(w, []minimax.Voice{{VoiceID: "id1", VoiceName: "Near"}})
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	_, err := resolveVoice(context.Background(), client, "nothing-match", "all", false)
	if err == nil {
		t.Fatalf("expected error for non-matching voice")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %q", err.Error())
	}
}

func TestResolveVoiceListOutputsTable(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeVoiceList(w, []minimax.Voice{{VoiceID: "id1", VoiceName: "Alpha"}})
	}))
	defer restoreHTTP()

	restore, read := captureStdout(t)
	defer restore()

	client := minimax.NewClient("key", "http://minimax.test")
	id, err := resolveVoice(context.Background(), client, "?", "all", false)
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != "" {
		t.Fatalf("expected empty ID when listing voices, got %q", id)
	}
	if out := read(); !strings.Contains(out, "VOICE ID") || !strings.Contains(out, "Alpha") {
		t.Fatalf("expected table output, got %q", out)
	}
}

func TestStreamAndPlayWritesOutput(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/t2a_v2" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeStreamResponse(w, "stream-bytes")
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	tmp := t.TempDir()
	out := tmp + "/out.mp3"
	opts := speakOptions{voiceID: "v1", outputPath: out, stream: true, play: false}
	payload := minimax.TTSRequest{Text: "hi", Stream: true, VoiceSetting: &minimax.VoiceSetting{VoiceID: "v1"}}

	if _, err := streamAndPlay(context.Background(), client, opts, payload); err != nil {
		t.Fatalf("streamAndPlay error: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "stream-bytes" {
		t.Fatalf("unexpected output data: %q", string(data))
	}
}

func TestConvertAndPlayWritesOutput(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/t2a_v2" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeJSONResponse(w, "convert-bytes")
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	tmp := t.TempDir()
	out := tmp + "/out.mp3"
	opts := speakOptions{voiceID: "v1", outputPath: out, play: false}
	payload := minimax.TTSRequest{Text: "hi", VoiceSetting: &minimax.VoiceSetting{VoiceID: "v1"}}

	if _, err := convertAndPlay(context.Background(), client, opts, payload); err != nil {
		t.Fatalf("convertAndPlay error: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "convert-bytes" {
		t.Fatalf("unexpected output data: %q", string(data))
	}
}

func TestStreamAndPlayRequiresWork(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeStreamResponse(w, "")
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://invalid")
	opts := speakOptions{voiceID: "v1", play: false, stream: true}
	payload := minimax.TTSRequest{Text: "hi", Stream: true, VoiceSetting: &minimax.VoiceSetting{VoiceID: "v1"}}

	_, err := streamAndPlay(context.Background(), client, opts, payload)
	if err == nil {
		t.Fatalf("expected error when no output and play disabled")
	}
}

func TestStreamAndPlayWithPlayback(t *testing.T) {
	called := false
	restore := stubPlay(t, func(data []byte) {
		called = true
		if string(data) != "stream-play" {
			t.Fatalf("unexpected data: %q", string(data))
		}
	})
	defer restore()

	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeStreamResponse(w, "stream-play")
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	opts := speakOptions{voiceID: "v1", play: true, stream: true}
	payload := minimax.TTSRequest{Text: "hi", Stream: true, VoiceSetting: &minimax.VoiceSetting{VoiceID: "v1"}}

	if _, err := streamAndPlay(context.Background(), client, opts, payload); err != nil {
		t.Fatalf("streamAndPlay error: %v", err)
	}
	if !called {
		t.Fatalf("expected playback to be invoked")
	}
}

func TestConvertAndPlayWithPlayback(t *testing.T) {
	called := false
	restore := stubPlay(t, func(data []byte) {
		called = true
		if string(data) != "convert-play" {
			t.Fatalf("unexpected data: %q", string(data))
		}
	})
	defer restore()

	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(w, "convert-play")
	}))
	defer restoreHTTP()

	client := minimax.NewClient("key", "http://minimax.test")
	opts := speakOptions{voiceID: "v1", play: true, outputPath: "", stream: false}
	payload := minimax.TTSRequest{Text: "hi", VoiceSetting: &minimax.VoiceSetting{VoiceID: "v1"}}

	if _, err := convertAndPlay(context.Background(), client, opts, payload); err != nil {
		t.Fatalf("convertAndPlay error: %v", err)
	}
	if !called {
		t.Fatalf("expected playback to be invoked")
	}
}

func captureStdout(t *testing.T) (restore func(), read func() string) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	return func() {
			_ = w.Close()
			os.Stdout = orig
		}, func() string {
			_ = w.Close()
			b, _ := io.ReadAll(r)
			return string(b)
		}
}

func captureStderr(t *testing.T) (restore func(), read func() string) {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	return func() {
			_ = w.Close()
			os.Stderr = orig
		}, func() string {
			_ = w.Close()
			b, _ := io.ReadAll(r)
			return string(b)
		}
}

func stubPlay(t *testing.T, fn func([]byte)) func() {
	t.Helper()
	orig := playToSpeakers
	playToSpeakers = func(_ context.Context, r io.Reader) error {
		b, _ := io.ReadAll(r)
		fn(b)
		return nil
	}
	return func() { playToSpeakers = orig }
}

func withMinimaxHandler(t *testing.T, handler http.Handler) func() {
	t.Helper()
	minimax.SetHTTPClient(&http.Client{Transport: handlerRoundTripper{handler: handler}})
	return func() { minimax.SetHTTPClient(nil) }
}

func writeVoiceList(w http.ResponseWriter, system []minimax.Voice) {
	resp := struct {
		SystemVoice     []minimax.Voice   `json:"system_voice"`
		VoiceCloning    []minimax.Voice   `json:"voice_cloning"`
		VoiceGeneration []minimax.Voice   `json:"voice_generation"`
		BaseResp        *minimax.BaseResp `json:"base_resp"`
	}{
		SystemVoice: system,
		BaseResp:    &minimax.BaseResp{StatusCode: 0, StatusMsg: "success"},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func writeStreamResponse(w http.ResponseWriter, payload string) {
	w.Header().Set("Content-Type", "text/event-stream")
	item := minimax.TTSResponse{
		Data:     &minimax.TTSResponseData{Audio: hex.EncodeToString([]byte(payload)), Status: 1},
		BaseResp: &minimax.BaseResp{StatusCode: 0, StatusMsg: "success"},
	}
	b, _ := json.Marshal(item)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
}

func writeJSONResponse(w http.ResponseWriter, payload string) {
	item := minimax.TTSResponse{
		Data:     &minimax.TTSResponseData{Audio: hex.EncodeToString([]byte(payload)), Status: 2},
		BaseResp: &minimax.BaseResp{StatusCode: 0, StatusMsg: "success"},
	}
	_ = json.NewEncoder(w).Encode(item)
}
