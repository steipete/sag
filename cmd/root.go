package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type rootConfig struct {
	APIKey  string
	BaseURL string
}

var (
	cfg         rootConfig
	versionFlag bool
	rootCmd     = &cobra.Command{
		Use:     "sag",
		Short:   "Command-line ElevenLabs TTS with macOS playback",
		Long:    "Command-line ElevenLabs TTS with macOS playback. Call it like macOS 'say': if you skip the subcommand, text args are passed to 'speak' (e.g. `sag \"Hello\"`).",
		Example: "  sag \"Hi Peter\"\n  echo 'piped input' | sag\n  sag speak -v Roger --rate 200 \"Faster speech\"",
		Version: "0.1.0",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if versionFlag {
				fmt.Println(cmd.Root().Name(), cmd.Root().Version)
				os.Exit(0)
			}
			return nil
		},
	}
)

// Execute is the entry point from main.
func Execute() {
	maybeDefaultToSpeak()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfg.APIKey, "api-key", "", "ElevenLabs API key (or ELEVENLABS_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&cfg.BaseURL, "base-url", "https://api.elevenlabs.io", "Override ElevenLabs API base URL")
	rootCmd.PersistentFlags().BoolVarP(&versionFlag, "version", "V", false, "Print version and exit")
}

// maybeDefaultToSpeak injects the "speak" subcommand when the user calls `sag` like macOS `say`.
func maybeDefaultToSpeak() {
	if len(os.Args) <= 1 {
		return
	}

	// npm/pnpm pass-through typically prefixes args with "--"; drop it so flags still parse.
	if os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		if len(os.Args) <= 1 {
			return
		}
	}

	first := os.Args[1]
	if isKnownSubcommand(first) || first == "-h" || first == "--help" {
		return
	}
	os.Args = append([]string{os.Args[0], "speak"}, os.Args[1:]...)
}

func isKnownSubcommand(name string) bool {
	name = strings.ToLower(name)
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			return true
		}
		for _, a := range cmd.Aliases {
			if a == name {
				return true
			}
		}
	}
	return false
}
