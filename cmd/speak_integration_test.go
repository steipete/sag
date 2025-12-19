package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
)

func TestSpeakCommand_FlagsBuildRequestAndMetrics(t *testing.T) {
	t.Helper()

	const voiceID = "abc1234567890123"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/text-to-speech/") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if path.Base(r.URL.Path) != voiceID {
			t.Fatalf("expected voice ID %q, got %q", voiceID, path.Base(r.URL.Path))
		}

		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if got["model_id"] != "eleven_v3" {
			t.Fatalf("expected model_id eleven_v3, got %v", got["model_id"])
		}
		if got["output_format"] != "mp3_44100_128" {
			t.Fatalf("expected output_format mp3_44100_128, got %v", got["output_format"])
		}
		if got["seed"] != float64(42) {
			t.Fatalf("expected seed 42, got %v", got["seed"])
		}
		if got["apply_text_normalization"] != "auto" {
			t.Fatalf("expected apply_text_normalization auto, got %v", got["apply_text_normalization"])
		}
		if got["language_code"] != "en" {
			t.Fatalf("expected language_code en, got %v", got["language_code"])
		}

		vs, ok := got["voice_settings"].(map[string]any)
		if !ok {
			t.Fatalf("expected voice_settings object, got %T", got["voice_settings"])
		}
		if vs["stability"] != 0.5 {
			t.Fatalf("expected stability 0.5, got %v", vs["stability"])
		}
		if vs["similarity_boost"] != 0.8 {
			t.Fatalf("expected similarity_boost 0.8, got %v", vs["similarity_boost"])
		}
		if vs["style"] != 0.1 {
			t.Fatalf("expected style 0.1, got %v", vs["style"])
		}
		if vs["use_speaker_boost"] != true {
			t.Fatalf("expected use_speaker_boost true, got %v", vs["use_speaker_boost"])
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("audio-bytes"))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	outPath := tmp + "/out.mp3"

	restore, read := captureStderr(t)
	defer restore()

	rootCmd.SetArgs([]string{
		"--api-key", "testkey",
		"--base-url", srv.URL,
		"speak",
		"--voice-id", voiceID,
		"--stream=false",
		"--play=false",
		"--output", outPath,
		"--metrics",
		"--stability", "0.5",
		"--similarity-boost", "0.8",
		"--style", "0.1",
		"--speaker-boost",
		"--seed", "42",
		"--normalize", "auto",
		"--lang", "EN",
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
