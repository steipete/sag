package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func resolveDeepgramKey() (string, error) {
	if key := strings.TrimSpace(os.Getenv("DEEPGRAM_API_KEY")); key != "" {
		return key, nil
	}
	if path := strings.TrimSpace(os.Getenv("DEEPGRAM_API_KEY_FILE")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read Deepgram api key file: %w", err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return "", fmt.Errorf("deepgram: api key file %q is empty", path)
		}
		return key, nil
	}
	return "", nil
}

func prepareDeepgramOptions(cmd *cobra.Command, opts *speakOptions) error {
	for _, flag := range []string{"model-id", "stability", "similarity", "similarity-boost", "style", "speaker-boost", "no-speaker-boost", "seed", "normalize", "lang", "latency-tier"} {
		if cmd.Flags().Changed(flag) {
			return fmt.Errorf("deepgram: does not support --%s; select an Aura model with --voice", flag)
		}
	}
	if !(opts.speed >= 0.7 && opts.speed <= 1.5) {
		return fmt.Errorf("deepgram: speed must be between 0.7 and 1.5")
	}
	if cmd.Flags().Changed("format") {
		if !strings.EqualFold(opts.outputFmt, "wav") {
			return fmt.Errorf("deepgram: supports wav output only; got %s", opts.outputFmt)
		}
	} else if ext := strings.ToLower(filepath.Ext(opts.outputPath)); ext != "" && ext != ".wav" {
		return fmt.Errorf("deepgram: supports .wav output only; got %s", ext)
	}
	opts.outputFmt = "wav"
	return preparePCMPlayback(cmd, opts, "Deepgram")
}
