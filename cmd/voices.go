package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/steipete/sag/internal/minimax"

	"github.com/spf13/cobra"
)

type voicesOptions struct {
	search string
	limit  int
}

func init() {
	opts := voicesOptions{
		limit: 100,
	}

	cmd := &cobra.Command{
		Use:   "voices",
		Short: "List available MiniMax voices",
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return ensureAPIKey()
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := minimax.NewClient(cfg.APIKey, cfg.BaseURL)
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			voices, err := client.ListVoices(ctx, "")
			if err != nil {
				return err
			}
			if opts.search != "" {
				voices = filterVoicesByName(voices, opts.search)
			}

			if opts.limit > 0 && len(voices) > opts.limit {
				voices = voices[:opts.limit]
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintf(w, "VOICE ID\tNAME\tCATEGORY\n"); err != nil {
				return err
			}
			for _, v := range voices {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", v.VoiceID, voiceLabel(v), v.Category); err != nil {
					return err
				}
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&opts.search, "search", "", "Filter voices by name or ID (client-side)")
	cmd.Flags().IntVar(&opts.limit, "limit", opts.limit, "Maximum rows to display (0 = all)")
	rootCmd.AddCommand(cmd)
}

func filterVoicesByName(voices []minimax.Voice, search string) []minimax.Voice {
	searchLower := strings.ToLower(search)
	filtered := make([]minimax.Voice, 0, len(voices))
	for _, v := range voices {
		if strings.Contains(strings.ToLower(voiceLabel(v)), searchLower) || strings.Contains(strings.ToLower(v.VoiceID), searchLower) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}
