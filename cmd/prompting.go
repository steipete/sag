package cmd

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed prompting_guide.md
var promptingGuide string

func init() {
	cmd := &cobra.Command{
		Use:     "prompting",
		Aliases: []string{"prompt", "guide", "tips"},
		Short:   "Prompting guide for better MiniMax speech",
		Long:    "Prints a practical prompting guide (model-specific tips, tags, and knobs) to improve voice quality and control.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := strings.TrimSpace(promptingGuide)
			_, err := fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	rootCmd.AddCommand(cmd)
}
