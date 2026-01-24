package cmd

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/steipete/sag/internal/minimax"
)

func TestVoicesCommand(t *testing.T) {
	restoreHTTP := withMinimaxHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/get_voice" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"system_voice":[{"voice_id":"id1","voice_name":"Alpha"}],"voice_cloning":[],"voice_generation":[],"base_resp":{"status_code":0,"status_msg":"success"}}`))
	}))
	defer restoreHTTP()

	cfg.APIKey = "key"
	cfg.BaseURL = "http://minimax.test"

	restore, readOut := captureStdoutVoices(t)
	defer restore()

	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"voices", "--limit", "1"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd execute: %v", err)
	}

	out := buf.String() + readOut()
	if !bytes.Contains([]byte(out), []byte("VOICE ID")) {
		t.Fatalf("expected table output, got %q", out)
	}

	// reset args to avoid polluting other tests
	rootCmd.SetArgs(nil)
	_ = os.Unsetenv("MINIMAX_API_KEY")
}

func TestFilterVoicesByName(t *testing.T) {
	voices := []minimax.Voice{
		{VoiceID: "id1", VoiceName: "Sarah"},
		{VoiceID: "id2", VoiceName: "Roger - Casual"},
		{VoiceID: "id3", VoiceName: "ROGUE"},
	}

	filtered := filterVoicesByName(voices, "rog")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(filtered))
	}
	if filtered[0].VoiceID != "id2" || filtered[1].VoiceID != "id3" {
		t.Fatalf("unexpected filter order: %+v", filtered)
	}
}

func captureStdoutVoices(t *testing.T) (restore func(), read func() string) {
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
