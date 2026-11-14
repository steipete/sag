package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steipete/sag/internal/chatgpt"
)

func TestChatGPTSelectionIsExplicitAndIsolated(t *testing.T) {
	resetProviderEnv(t)
	t.Setenv("SAG_CHATGPT_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")
	cfg.Provider = providerChatGPT
	// Unrelated API-key files must not be consulted by account authentication.
	t.Setenv("ELEVENLABS_API_KEY_FILE", filepath.Join(t.TempDir(), "missing"))
	got, err := selectProvider()
	if err != nil || got.name != providerChatGPT || got.chatgpt == nil {
		t.Fatalf("provider=%s err=%v", got.name, err)
	}
	t.Setenv("ELEVENLABS_API_KEY_FILE", "")
	cfg.Provider = ""
	got, err = selectProvider()
	if err != nil || got.name != providerOpenAI {
		t.Fatalf("ChatGPT login changed automatic selection: %s %v", got.name, err)
	}
	cfg.Provider = providerChatGPT
	cfg.BaseURL = "https://example.test"
	if _, err := selectProvider(); err == nil {
		t.Fatal("accepted an alternate host for account auth")
	}
}

func TestChatGPTOptions(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "", "")
	cmd.Flags().Bool("stream", true, "")
	cmd.Flags().String("model-id", "", "")
	cmd.Flags().Duration("timeout", 0, "")
	t.Setenv("SAG_TIMEOUT", "")
	t.Setenv("SAG_PLAYER", "")
	opts := speakOptions{stream: true, play: true, outputPath: "voice.wav"}
	if err := prepareChatGPTOptions(cmd, &opts); err != nil {
		t.Fatal(err)
	}
	if !opts.stream || opts.outputFmt != "wav" || opts.timeout != 90*time.Second {
		t.Fatalf("wrong ChatGPT defaults: %+v", opts)
	}
	if err := cmd.Flags().Set("stream", "true"); err != nil {
		t.Fatal(err)
	}
	opts.stream = true
	if err := prepareChatGPTOptions(cmd, &opts); err != nil {
		t.Fatal(err)
	}
	opts.player = "afplay"
	if err := prepareChatGPTOptions(cmd, &opts); err == nil {
		t.Fatal("accepted streaming through a buffered player")
	}
}

func TestChatGPTKeepAliveValidation(t *testing.T) {
	resetRootCommandState()
	t.Cleanup(resetRootCommandState)
	resetProviderEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-key")
	rootCmd.SetArgs([]string{"speak", "--provider", "openai", "--keep-alive", "1m", "hello"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires --provider chatgpt") {
		t.Fatalf("wrong provider accepted keep-alive: %v", err)
	}
	for _, duration := range []time.Duration{-time.Second, 0, 6 * time.Minute} {
		if err := chatgpt.ValidateKeepAlive(duration); err == nil {
			t.Fatalf("accepted lifetime %s", duration)
		}
	}
}

func TestLiveChatGPTPlayback(t *testing.T) {
	if os.Getenv("SAG_TEST_CHATGPT_PLAYBACK") != "1" {
		t.Skip("requires a prestarted voice worker and live-test opt-in")
	}
	resetProviderEnv(t)
	resetRootCommandState()
	t.Cleanup(resetRootCommandState)
	t.Setenv("SAG_PLAYER", "")
	t.Setenv("SAG_TIMEOUT", "")
	original := playPCM
	t.Cleanup(func() { playPCM = original })
	for i := range 2 {
		resetRootCommandState()
		start := time.Now()
		var first time.Duration
		playPCM = func(_ context.Context, r io.ReadCloser) error {
			var firstSamples [2]byte
			if _, err := io.ReadFull(r, firstSamples[:]); err != nil {
				return err
			}
			first = time.Since(start)
			_, err := io.Copy(io.Discard, r)
			return err
		}
		out := filepath.Join(t.TempDir(), "speech.wav")
		rootCmd.SetArgs([]string{"speak", "--provider", "chatgpt", "--keep-alive", "5s", "-v", "maple", "--metrics", "--play", "-o", out, "Maple, let's test the startup delay."})
		if err := rootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out)
		if err != nil || len(data) < 48000 || string(data[:4]) != "RIFF" {
			t.Fatalf("invalid completed WAV: %v", err)
		}
		t.Logf("CLI request %d: first playback data=%s, WAV bytes=%d", i+1, first, len(data))
		if first == 0 || i > 0 && first > 2*time.Second {
			t.Fatalf("unexpected playback delay: %s", first)
		}
	}
}
