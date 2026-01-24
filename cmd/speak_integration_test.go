package cmd

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/steipete/sag/internal/minimax"
)

func TestSpeakCommand_FlagsBuildRequestAndMetrics(t *testing.T) {
	t.Helper()

	const voiceID = "voice-123"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/t2a_v2" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if got["model"] != "speech-01" {
			t.Fatalf("expected model speech-01, got %v", got["model"])
		}
		if got["text"] != "Hello world" {
			t.Fatalf("expected text Hello world, got %v", got["text"])
		}
		if v, ok := got["stream"]; ok && v != false {
			t.Fatalf("expected stream false, got %v", v)
		}
		if got["language_boost"] != "en" {
			t.Fatalf("expected language_boost en, got %v", got["language_boost"])
		}
		if got["output_format"] != "mp3" {
			t.Fatalf("expected output_format mp3, got %v", got["output_format"])
		}

		voiceSettings, ok := got["voice_setting"].(map[string]any)
		if !ok {
			t.Fatalf("expected voice_setting object, got %T", got["voice_setting"])
		}
		if voiceSettings["voice_id"] != voiceID {
			t.Fatalf("expected voice_id %q, got %v", voiceID, voiceSettings["voice_id"])
		}
		if voiceSettings["text_normalization"] != true {
			t.Fatalf("expected text_normalization true, got %v", voiceSettings["text_normalization"])
		}
		if voiceSettings["emotion"] != "happy" {
			t.Fatalf("expected emotion happy, got %v", voiceSettings["emotion"])
		}
		if voiceSettings["pitch"] != float64(2) {
			t.Fatalf("expected pitch 2, got %v", voiceSettings["pitch"])
		}
		if voiceSettings["vol"] != 1.5 {
			t.Fatalf("expected vol 1.5, got %v", voiceSettings["vol"])
		}

		audioSetting, ok := got["audio_setting"].(map[string]any)
		if !ok {
			t.Fatalf("expected audio_setting object, got %T", got["audio_setting"])
		}
		if audioSetting["format"] != "mp3" {
			t.Fatalf("expected audio_setting.format mp3, got %v", audioSetting["format"])
		}
		if audioSetting["sample_rate"] != float64(44100) {
			t.Fatalf("expected audio_setting.sample_rate 44100, got %v", audioSetting["sample_rate"])
		}
		if audioSetting["bitrate"] != float64(128000) {
			t.Fatalf("expected audio_setting.bitrate 128000, got %v", audioSetting["bitrate"])
		}

		resp := map[string]any{
			"data": map[string]any{
				"audio":  hex.EncodeToString([]byte("audio-bytes")),
				"status": 2,
			},
			"base_resp": map[string]any{
				"status_code": 0,
				"status_msg":  "success",
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})

	minimax.SetHTTPClient(&http.Client{Transport: handlerRoundTripper{handler: handler}})
	defer minimax.SetHTTPClient(nil)

	tmp := t.TempDir()
	outPath := tmp + "/out.mp3"

	restore, read := captureStderr(t)
	defer restore()

	rootCmd.SetArgs([]string{
		"--api-key", "testkey",
		"--base-url", "http://minimax.test",
		"speak",
		"--voice-id", voiceID,
		"--stream=false",
		"--play=false",
		"--output", outPath,
		"--metrics",
		"--emotion", "happy",
		"--pitch", "2",
		"--volume", "1.5",
		"--normalize", "on",
		"--lang", "en",
		"Hello world",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("speak command failed: %v", err)
	}

	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file to be written: %v", err)
	}

	stderr := read()
	if !strings.Contains(stderr, "metrics: chars=") || !strings.Contains(stderr, "bytes=") || !strings.Contains(stderr, "dur=") {
		t.Fatalf("expected metrics output, got %q", stderr)
	}
}
