package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeepgramExplicitSelection(t *testing.T) {
	resetProviderEnv(t)
	t.Setenv("DEEPGRAM_API_KEY", "test-key")
	if _, err := selectProvider(); err == nil {
		t.Fatal("Deepgram must not silently become the default")
	}
	cfg.Provider = providerDeepgram
	p, err := selectProvider()
	if err != nil || p.deepgram == nil {
		t.Fatalf("Deepgram selection: %v", err)
	}
	cfg.Provider = ""
	t.Setenv("ELEVENLABS_API_KEY", "existing-key")
	p, err = selectProvider()
	if err != nil || p.name != providerElevenLabs {
		t.Fatalf("existing default changed: %v", err)
	}
}

func TestSpeakDeepgramStreamingAndSave(t *testing.T) {
	resetProviderEnv(t)
	resetRootCommandState()
	t.Cleanup(resetRootCommandState)
	t.Setenv("DEEPGRAM_API_KEY", "test-key")
	t.Setenv("SAG_PLAYER", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/speak" || r.URL.Query().Get("model") != "aura-2-thalia-en" {
			t.Errorf("unexpected endpoint %s", r.URL)
		}
		w.Header().Set("Content-Type", "audio/l16;rate=24000")
		_, _ = w.Write([]byte{1, 2, 3, 4})
	}))
	defer srv.Close()
	old := playPCM
	t.Cleanup(func() { playPCM = old })
	var played []byte
	playPCM = func(_ context.Context, r io.ReadCloser) error { var err error; played, err = io.ReadAll(r); return err }
	out := filepath.Join(t.TempDir(), "speech.wav")
	rootCmd.SetArgs([]string{"speak", "--provider", "deepgram", "--base-url", srv.URL, "--play", "-o", out, "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wav, err := os.ReadFile(out)
	if err != nil || len(wav) != 48 || !bytes.Equal(played, wav[44:]) {
		t.Fatalf("playback/file mismatch: %v", err)
	}
}

func TestDeepgramUnsupportedOptions(t *testing.T) {
	for _, args := range [][]string{{"--format", "mp3"}, {"-o", "out.mp3"}, {"--speed", "0.6"}, {"--speed", "1.6"}, {"--model-id", "model"}, {"--seed", "1"}, {"--lang", "en"}, {"--stream", "--player", "afplay"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			resetProviderEnv(t)
			resetRootCommandState()
			t.Cleanup(resetRootCommandState)
			t.Setenv("DEEPGRAM_API_KEY", "test-key")
			rootCmd.SetArgs(append([]string{"speak", "--provider", "deepgram", "--base-url", "http://unused.invalid", "hello"}, args...))
			if err := rootCmd.Execute(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "deepgram") {
				t.Fatalf("expected local option error: %v", err)
			}
		})
	}
}
