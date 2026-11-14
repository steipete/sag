package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
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
	opts := speakOptions{stream: true, play: true, outputPath: "voice.wav"}
	if err := prepareChatGPTOptions(cmd, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.stream || opts.outputFmt != "wav" || opts.timeout != 90*time.Second {
		t.Fatalf("wrong ChatGPT defaults: %+v", opts)
	}
	if err := cmd.Flags().Set("stream", "true"); err != nil {
		t.Fatal(err)
	}
	opts.stream = true
	if err := prepareChatGPTOptions(cmd, &opts); err == nil {
		t.Fatal("accepted unsupported streaming")
	}
}
