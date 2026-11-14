package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steipete/sag/internal/chatgpt"
)

func init() {
	var device bool
	login := &cobra.Command{
		Use: "login", Short: "Sign in to ChatGPT using separate sag credentials",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.ToLower(cfg.Provider) != providerChatGPT {
				return fmt.Errorf("use sag login --provider chatgpt; API-key providers use their existing credential settings")
			}
			client, err := chatgpt.NewClient()
			if err != nil {
				return err
			}
			return client.Login(cmd.Context(), device, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	login.Flags().BoolVar(&device, "device-auth", false, "Sign in using an OpenAI browser link and one-time device code")
	rootCmd.AddCommand(login)
	var idle time.Duration
	worker := &cobra.Command{Use: "voice-worker", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := chatgpt.NewClient()
		if err != nil {
			return err
		}
		return client.ServeWarm(cmd.Context(), idle)
	}}
	worker.Flags().DurationVar(&idle, "idle-timeout", 30*time.Second, "Idle lifetime of the local voice worker")
	rootCmd.AddCommand(worker)
}

func prepareChatGPTOptions(cmd *cobra.Command, opts *speakOptions) error {
	for _, flag := range []string{"model-id", "speed", "rate", "stability", "similarity", "similarity-boost", "style", "speaker-boost", "no-speaker-boost", "seed", "normalize", "lang", "latency-tier"} {
		if cmd.Flags().Changed(flag) {
			return fmt.Errorf("ChatGPT voice does not support --%s", flag)
		}
	}
	if cmd.Flags().Changed("format") && !strings.EqualFold(opts.outputFmt, "wav") {
		return fmt.Errorf("ChatGPT voice supports wav output only")
	}
	if ext := strings.ToLower(filepath.Ext(opts.outputPath)); ext != "" && ext != ".wav" {
		return fmt.Errorf("ChatGPT voice supports .wav output only")
	}
	opts.outputFmt = "wav"
	if !cmd.Flags().Changed("timeout") && strings.TrimSpace(os.Getenv("SAG_TIMEOUT")) == "" {
		opts.timeout = 90 * time.Second
	}
	return preparePCMPlayback(cmd, opts, "ChatGPT")
}
