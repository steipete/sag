package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/steipete/sag/internal/audio"
	"github.com/steipete/sag/internal/tts"

	"github.com/spf13/cobra"
)

type voicesOptions struct {
	search string
	query  string
	labels []string
	limit  int
	try    bool
}

var (
	previewHTTPClient = &http.Client{Timeout: 45 * time.Second}
	playVoicePreview  = playVoicePreviewImpl
)

func init() {
	opts := voicesOptions{
		limit: 100,
	}

	cmd := &cobra.Command{
		Use:   "voices",
		Short: "List voices for the configured TTS provider",
		RunE: func(cmd *cobra.Command, _ []string) error {
			hasLabelFilters := false
			for _, label := range opts.labels {
				if strings.TrimSpace(label) != "" {
					hasLabelFilters = true
					break
				}
			}
			if opts.try && opts.search == "" && opts.query == "" && !hasLabelFilters && !cmd.Flags().Changed("limit") {
				return errors.New("--try requires --search, --query, --label, or --limit to avoid playing all voices")
			}

			provider, err := selectProvider()
			if err != nil {
				return err
			}
			client := provider.voices
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			var voices []tts.Voice
			if opts.search != "" {
				voices, err = client.SearchVoices(ctx, opts.search, opts.limit)
				if err != nil {
					voices, err = client.ListVoices(ctx)
					if err != nil {
						return err
					}
					voices = filterVoicesByName(voices, opts.search)
				}
			} else {
				voices, err = client.ListVoices(ctx)
				if err != nil {
					return err
				}
			}

			labelFilters, err := parseLabelFilters(opts.labels)
			if err != nil {
				return err
			}

			needsMeta := opts.query != "" || len(labelFilters) > 0 || opts.try
			if needsMeta {
				cachePath, cacheErr := voiceCachePath()
				if cacheErr != nil {
					fmt.Fprintf(os.Stderr, "warning: voice cache disabled: %v\n", cacheErr)
				}
				cache := newVoiceCache()
				if cacheErr == nil {
					loaded, err := loadVoiceCache(cachePath)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: failed to load voice cache: %v\n", err)
					} else {
						cache = loaded
					}
				}

				var metaCount int
				voices, metaCount = hydrateVoices(cmd.Context(), client, voices, cache, voiceCacheTTL)
				if metaCount == 0 && (opts.query != "" || len(labelFilters) > 0) {
					fmt.Fprintln(os.Stderr, "warning: voice metadata unavailable; matching on names only")
				}

				if cacheErr == nil {
					if err := saveVoiceCache(cachePath, cache); err != nil {
						fmt.Fprintf(os.Stderr, "warning: failed to save voice cache: %v\n", err)
					}
				}
			}

			if len(labelFilters) > 0 {
				voices = filterVoicesByLabels(voices, labelFilters)
				if len(voices) == 0 {
					return errors.New("no voices matched label filters")
				}
			}

			if opts.query != "" {
				voices = rankVoicesByQuery(voices, opts.query)
				if len(voices) == 0 {
					return fmt.Errorf("no voices matched query %q; try --search or adjust wording", opts.query)
				}
			}

			if opts.limit > 0 && len(voices) > opts.limit {
				voices = voices[:opts.limit]
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintf(w, "VOICE ID\tNAME\tCATEGORY\n"); err != nil {
				return err
			}
			for _, v := range voices {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", v.VoiceID, v.Name, v.Category); err != nil {
					return err
				}
			}
			if err := w.Flush(); err != nil {
				return err
			}

			if opts.try {
				if len(voices) == 0 {
					return errors.New("no voices to preview")
				}
				var previewed int
				for _, v := range voices {
					fmt.Fprintf(os.Stderr, "preview: %s (%s)\n", v.Name, v.VoiceID)
					if err := playVoicePreview(cmd.Context(), client, v); err != nil {
						fmt.Fprintf(os.Stderr, "preview failed for %s (%s): %v\n", v.Name, v.VoiceID, err)
						continue
					}
					previewed++
				}
				if previewed == 0 {
					return errors.New("no previews played")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.search, "search", "", "Search voices by name (server-side when supported)")
	cmd.Flags().StringVar(&opts.query, "query", "", "Semantic query over name/description/labels (client-side)")
	cmd.Flags().StringArrayVar(&opts.labels, "label", nil, "Filter by voice label (key=value); repeatable")
	cmd.Flags().IntVar(&opts.limit, "limit", opts.limit, "Maximum rows to display (0 = all)")
	cmd.Flags().BoolVar(&opts.try, "try", false, "Play preview audio for listed voices (requires --search, --query, --label, or --limit)")
	rootCmd.AddCommand(cmd)
}

func filterVoicesByName(voices []tts.Voice, search string) []tts.Voice {
	searchLower := strings.ToLower(search)
	filtered := make([]tts.Voice, 0, len(voices))
	for _, v := range voices {
		if strings.Contains(strings.ToLower(v.Name), searchLower) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func playVoicePreviewImpl(ctx context.Context, client tts.VoiceCatalog, voice tts.Voice) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	previewURL := strings.TrimSpace(voice.PreviewURL)
	if previewURL == "" && voice.VoiceID != "" {
		voiceDetails, err := client.GetVoice(ctx, voice.VoiceID)
		if err != nil {
			return err
		}
		previewURL = strings.TrimSpace(voiceDetails.PreviewURL)
	}
	if previewURL == "" {
		return errors.New("preview URL unavailable")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, previewURL, nil)
	if err != nil {
		return err
	}
	resp, err := previewHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("preview download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return audio.StreamToSpeakers(ctx, resp.Body)
}
