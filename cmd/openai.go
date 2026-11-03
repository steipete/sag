package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steipete/sag/internal/openai"
)

func prepareOpenAIOptions(cmd *cobra.Command, opts *speakOptions) error {
	for _, flag := range []string{"stability", "similarity", "similarity-boost", "style", "speaker-boost", "no-speaker-boost", "seed", "normalize", "lang", "latency-tier"} {
		if cmd.Flags().Changed(flag) {
			return fmt.Errorf("OpenAI Realtime does not support --%s", flag)
		}
	}
	if !cmd.Flags().Changed("model-id") {
		opts.modelID = openai.DefaultModel
	}
	if !(opts.speed >= 0.5 && opts.speed <= 1.5) {
		return fmt.Errorf("OpenAI speed must be between 0.5 and 1.5")
	}
	if cmd.Flags().Changed("format") {
		if !strings.EqualFold(opts.outputFmt, "wav") {
			return fmt.Errorf("OpenAI Realtime supports wav output only; got %s", opts.outputFmt)
		}
	} else if ext := strings.ToLower(filepath.Ext(opts.outputPath)); ext != "" && ext != ".wav" {
		return fmt.Errorf("OpenAI Realtime supports .wav output only; got %s", ext)
	}
	opts.outputFmt = "wav"
	if opts.stream && cmd.Flags().Changed("stream") {
		return fmt.Errorf("OpenAI Realtime audio is validated before playback; omit --stream")
	}
	opts.stream = false
	return nil
}
