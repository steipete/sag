package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestOpenAIProviderSelection(t *testing.T) {
	for _, tc := range []struct {
		name, selected, el, sd, oa string
		want                       string
		wantError                  bool
	}{
		{"auto", "", "", "", "key", providerOpenAI, false},
		{"ambiguous ElevenLabs", "", "key", "", "key", "", true},
		{"ambiguous 60db", "", "", "key", "key", "", true},
		{"explicit OpenAI", "openai", "key", "key", "key", providerOpenAI, false},
		{"explicit ElevenLabs", "elevenlabs", "key", "key", "key", providerElevenLabs, false},
		{"explicit 60db", "60db", "key", "key", "key", providerSixtyDB, false},
		{"missing selected key", "openai", "key", "", "", "", true},
		{"unknown", "other", "", "", "key", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetProviderEnv(t)
			cfg.Provider = tc.selected
			t.Setenv("ELEVENLABS_API_KEY", tc.el)
			t.Setenv("SIXTYDB_API_KEY", tc.sd)
			t.Setenv("OPENAI_API_KEY", tc.oa)
			got, err := selectProvider()
			if (err != nil) != tc.wantError || got.name != tc.want {
				t.Fatalf("provider=%s err=%v", got.name, err)
			}
		})
	}
}

func TestOpenAIKeyFile(t *testing.T) {
	resetProviderEnv(t)
	cfg.Provider = providerOpenAI
	p := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(p, []byte(" file-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY_FILE", p)
	key, err := resolveOpenAIKey()
	if err != nil || key != "file-key" {
		t.Fatalf("key file error: %v", err)
	}
	t.Setenv("OPENAI_API_KEY", " env-key ")
	t.Setenv("OPENAI_API_KEY_FILE", p+"-missing")
	key, err = resolveOpenAIKey()
	if err != nil || key != "env-key" {
		t.Fatalf("env precedence: %v", err)
	}
	// A selected provider must not read another provider's stale key files.
	t.Setenv("ELEVENLABS_API_KEY_FILE", p+"-missing")
	if _, err := selectProvider(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := resolveOpenAIKey(); err == nil {
		t.Fatal("missing file accepted")
	}
	t.Setenv("OPENAI_API_KEY_FILE", p)
	if err := os.WriteFile(p, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOpenAIKey(); err == nil {
		t.Fatal("empty file accepted")
	}
}

func TestSpeakOpenAIPlaybackAndFile(t *testing.T) {
	resetProviderEnv(t)
	resetRootCommandState()
	t.Cleanup(resetRootCommandState)
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("SAG_PLAYER", "")
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/realtime" || r.URL.Query().Get("model") != "gpt-realtime-2.1-mini" {
			t.Errorf("unexpected endpoint %s", r.URL)
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		for _, event := range []string{"session.created", "session.updated"} {
			if err := wsjson.Write(ctx, conn, map[string]any{"type": event}); err != nil {
				t.Error(err)
				return
			}
			var request json.RawMessage
			if err := wsjson.Read(ctx, conn, &request); err != nil {
				t.Error(err)
				return
			}
		}
		for _, event := range []string{
			`{"type":"response.output_audio.delta","delta":"AQIDBA=="}`,
			`{"type":"response.output_audio.done"}`,
			`{"type":"response.done","response":{"status":"completed"}}`,
		} {
			if err := conn.Write(ctx, websocket.MessageText, []byte(event)); err != nil {
				t.Error(err)
				return
			}
		}
	}))
	defer srv.Close()
	var played []byte
	original := playToSpeakers
	playToSpeakers = func(_ context.Context, r io.Reader) error { var err error; played, err = io.ReadAll(r); return err }
	t.Cleanup(func() { playToSpeakers = original })
	out := filepath.Join(t.TempDir(), "test.wav")
	rootCmd.SetArgs([]string{"speak", "--provider", "openai", "--base-url", srv.URL + "/v1", "-v", "marin", "--player", "auto", "--play", "-o", out, "--timeout", "5s", "Hello world"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(saved) != 48 || string(saved[:4]) != "RIFF" || string(saved) != string(played) {
		t.Fatalf("requests=%d saved=%d played=%d", requests, len(saved), len(played))
	}
}

func TestSpeakOpenAIRejectsUnsupportedOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--format", "mp3"}, {"-o", "out.mp3"}, {"-o", "out.flac"}, {"--stream"}, {"--speed", "1.6"}, {"--rate", "300"},
		{"--stability", "0.5"}, {"--similarity", "0.5"}, {"--similarity-boost", "0.5"}, {"--style", "0.5"}, {"--speaker-boost"}, {"--no-speaker-boost"}, {"--seed", "1"}, {"--normalize", "auto"}, {"--lang", "en"}, {"--latency-tier", "1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			resetProviderEnv(t)
			resetRootCommandState()
			t.Cleanup(resetRootCommandState)
			t.Setenv("OPENAI_API_KEY", "test-key")
			rootCmd.SetArgs(append([]string{"speak", "--provider", "openai", "--base-url", "http://unused.invalid", "hello"}, args...))
			err := rootCmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "OpenAI") {
				t.Fatalf("expected local validation error, got %v", err)
			}
		})
	}
}
