package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

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
