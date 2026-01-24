package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/steipete/sag/internal/audio"
	"github.com/steipete/sag/internal/minimax"

	"github.com/spf13/cobra"
)

type speakOptions struct {
	voiceID       string
	voiceCategory string
	modelID       string
	outputPath    string
	outputFmt     string
	stream        bool
	play          bool
	speed         float64
	rateWPM       int
	inputFile     string
	normalize     string
	lang          string
	metrics       bool

	emotion string
	pitch   int
	volume  float64
}

const defaultWPM = 175 // matches macOS `say` default rate

var playToSpeakers = audio.StreamToSpeakers

func init() {
	opts := speakOptions{
		modelID:       "speech-01",
		outputFmt:     "mp3_44100_128",
		stream:        true,
		play:          true,
		speed:         1.0,
		voiceCategory: "all",
	}

	cmd := &cobra.Command{
		Use:   "speak [text]",
		Short: "Speak the provided text using MiniMax TTS (default: stream to speakers)",
		Long:  "If no text argument is provided, the command reads from stdin.\n\nTip: run `sag prompting` for model-specific prompting tips and recommended flag combinations.",
		Args:  cobra.ArbitraryArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return ensureAPIKey()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := applyRateAndSpeed(&opts); err != nil {
				return err
			}

			forceVoiceID := cmd.Flags().Changed("voice-id")
			voiceInput := opts.voiceID
			if voiceInput == "" {
				if env := os.Getenv("MINIMAX_VOICE_ID"); env != "" {
					voiceInput = env
					forceVoiceID = true
				} else if env := os.Getenv("SAG_VOICE_ID"); env != "" {
					voiceInput = env
					forceVoiceID = true
				}
			}
			client := minimax.NewClient(cfg.APIKey, cfg.BaseURL)

			category, err := normalizeVoiceCategory(opts.voiceCategory)
			if err != nil {
				return err
			}

			voiceID, err := resolveVoice(cmd.Context(), client, voiceInput, category, forceVoiceID)
			if err != nil {
				return err
			}
			if voiceID == "" {
				// Likely printed voices for '?' request.
				return nil
			}
			opts.voiceID = voiceID

			text, err := resolveText(args, opts.inputFile)
			if err != nil {
				return err
			}

			// If user provided output path with a known extension, infer a compatible format.
			if opts.outputPath != "" {
				if inferred := inferFormatFromExt(opts.outputPath); inferred != "" {
					opts.outputFmt = inferred
				}
				// Disable playback when -o is set, unless --play was explicitly provided
				if !cmd.Flags().Changed("play") {
					opts.play = false
				}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 90*time.Second)
			defer cancel()

			payload, err := buildTTSRequest(cmd, opts, text)
			if err != nil {
				return err
			}

			if opts.play {
				formatKind := outputFormatKind(opts.outputFmt)
				if formatKind != "" && formatKind != "mp3" {
					return fmt.Errorf("playback requires mp3 output; use --format mp3 or disable --play")
				}
				if formatKind == "" && opts.outputFmt != "" {
					return fmt.Errorf("playback requires mp3 output; unknown format %q", opts.outputFmt)
				}
			}

			start := time.Now()
			var bytes int64
			if opts.stream {
				n, err := streamAndPlay(ctx, client, opts, payload)
				bytes = n
				if err != nil {
					return err
				}
			} else {
				n, err := convertAndPlay(ctx, client, opts, payload)
				bytes = n
				if err != nil {
					return err
				}
			}
			if opts.metrics {
				fmt.Fprintf(os.Stderr, "metrics: chars=%d bytes=%d model=%s voice=%s stream=%t dur=%s\n",
					len([]rune(text)), bytes, opts.modelID, opts.voiceID, opts.stream, time.Since(start).Truncate(time.Millisecond))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.voiceID, "voice-id", "", "Voice ID to use (MINIMAX_VOICE_ID)")
	cmd.Flags().StringVarP(&opts.voiceID, "voice", "v", "", "Alias for --voice-id; accepts name or ID; use '?' to list voices")
	cmd.Flags().StringVar(&opts.voiceCategory, "voice-category", opts.voiceCategory, "Voice category to query (system|voice_cloning|voice_generation|all)")
	cmd.Flags().StringVar(&opts.modelID, "model-id", opts.modelID, "Model ID (default: speech-01). See MiniMax docs for available models.")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "Write audio to file (disables playback unless --play is also set)")
	cmd.Flags().StringVar(&opts.outputFmt, "format", opts.outputFmt, "Output format (e.g. mp3_44100_128)")
	cmd.Flags().BoolVar(&opts.stream, "stream", opts.stream, "Stream audio while generating")
	cmd.Flags().BoolVar(&opts.play, "play", opts.play, "Play audio through speakers")
	cmd.Flags().Float64Var(&opts.speed, "speed", opts.speed, "Speech speed multiplier (e.g. 1.1 faster, 0.9 slower)")
	cmd.Flags().IntVarP(&opts.rateWPM, "rate", "r", 0, "macOS say-style words-per-minute; overrides --speed when set (default 175 wpm)")
	cmd.Flags().StringVar(&opts.normalize, "normalize", "", "Text normalization: auto|on|off (auto = server default; when set)")
	cmd.Flags().StringVar(&opts.lang, "lang", "", "Language boost hint (e.g. en, zh, auto; when set)")
	cmd.Flags().StringVar(&opts.emotion, "emotion", "", "Emotion hint (model dependent; e.g. neutral, happy, sad)")
	cmd.Flags().IntVar(&opts.pitch, "pitch", 0, "Pitch adjustment (model dependent; when set)")
	cmd.Flags().Float64Var(&opts.volume, "volume", 0, "Volume adjustment (model dependent; when set)")
	cmd.Flags().BoolVar(&opts.metrics, "metrics", false, "Print request metrics to stderr (chars, bytes, duration, etc.)")
	cmd.Flags().StringVarP(&opts.inputFile, "input-file", "f", "", "Read text from file (use '-' for stdin), matching macOS say -f")
	cmd.Flags().Bool("progress", false, "Accepted for macOS say compatibility (no-op)")
	cmd.Flags().String("network-send", "", "Accepted for macOS say compatibility (not implemented)")
	cmd.Flags().String("audio-device", "", "Accepted for macOS say compatibility (not implemented)")
	cmd.Flags().String("interactive", "", "Accepted for macOS say compatibility (not implemented)")
	cmd.Flags().String("file-format", "", "Accepted for macOS say compatibility (not implemented)")
	cmd.Flags().String("data-format", "", "Accepted for macOS say compatibility (not implemented)")
	cmd.Flags().Int("channels", 0, "Accepted for macOS say compatibility (not implemented)")
	cmd.Flags().Int("bit-rate", 0, "Accepted for macOS say compatibility (not implemented)")
	cmd.Flags().Int("quality", 0, "Accepted for macOS say compatibility (not implemented)")

	rootCmd.AddCommand(cmd)
}

func applyRateAndSpeed(opts *speakOptions) error {
	if opts.rateWPM > 0 {
		// Map macOS `say` rate (words per minute) to speed multiplier.
		opts.speed = float64(opts.rateWPM) / float64(defaultWPM)
		if opts.speed <= 0.5 || opts.speed >= 2.0 {
			return fmt.Errorf("rate %d wpm maps to speed %.2f, which is outside the allowed 0.5–2.0 range", opts.rateWPM, opts.speed)
		}
		return nil
	}
	if opts.speed <= 0.5 || opts.speed >= 2.0 {
		return errors.New("speed must be between 0.5 and 2.0 (e.g. 1.1 for 10% faster)")
	}
	return nil
}

func buildTTSRequest(cmd *cobra.Command, opts speakOptions, text string) (minimax.TTSRequest, error) {
	flags := cmd.Flags()

	normalize := strings.ToLower(strings.TrimSpace(opts.normalize))
	var textNormalization *bool
	if flags.Changed("normalize") {
		switch normalize {
		case "auto":
		case "on":
			v := true
			textNormalization = &v
		case "off":
			v := false
			textNormalization = &v
		default:
			return minimax.TTSRequest{}, errors.New("normalize must be one of: auto, on, off")
		}
	}

	lang := strings.TrimSpace(opts.lang)
	if flags.Changed("lang") {
		if lang == "" {
			return minimax.TTSRequest{}, errors.New("lang must be non-empty when set")
		}
	} else {
		lang = ""
	}

	outputFormat, audioSetting, err := parseOutputFormat(opts.outputFmt)
	if err != nil {
		return minimax.TTSRequest{}, err
	}

	speed := opts.speed
	voice := &minimax.VoiceSetting{
		VoiceID:           opts.voiceID,
		Speed:             &speed,
		TextNormalization: textNormalization,
	}
	if flags.Changed("volume") {
		v := opts.volume
		voice.Vol = &v
	}
	if flags.Changed("pitch") {
		v := opts.pitch
		voice.Pitch = &v
	}
	if flags.Changed("emotion") {
		voice.Emotion = strings.TrimSpace(opts.emotion)
	}

	req := minimax.TTSRequest{
		Model:         opts.modelID,
		Text:          text,
		Stream:        opts.stream,
		VoiceSetting:  voice,
		AudioSetting:  audioSetting,
		OutputFormat:  outputFormat,
		LanguageBoost: lang,
	}
	if opts.stream {
		req.StreamOptions = &minimax.StreamOptions{ExcludeAggregatedAudio: true}
	}
	return req, nil
}

func resolveText(args []string, inputFile string) (string, error) {
	if inputFile != "" {
		if inputFile == "-" {
			return readStdin()
		}
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return "", err
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return "", errors.New("input file was empty")
		}
		return text, nil
	}

	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	return readStdin()
}

func readStdin() (string, error) {
	if isStdinTTY() {
		return "", errors.New("no text provided; pass text args, --input-file, or pipe input")
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return "", errors.New("stdin was empty")
	}
	return text, nil
}

func isStdinTTY() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func streamAndPlay(ctx context.Context, client *minimax.Client, opts speakOptions, payload minimax.TTSRequest) (int64, error) {
	resp, err := client.StreamTTS(ctx, payload)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = resp.Close()
	}()

	writers := make([]io.Writer, 0, 2)
	var file io.WriteCloser
	if opts.outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.outputPath), 0o755); err != nil {
			return 0, err
		}
		file, err = os.Create(opts.outputPath)
		if err != nil {
			return 0, err
		}
		defer func() {
			_ = file.Close()
		}()
		writers = append(writers, file)
	}

	if opts.play {
		pr, pw := io.Pipe()
		writers = append(writers, pw)
		mw := io.MultiWriter(writers...)

		copyErr := make(chan error, 1)
		copyN := make(chan int64, 1)
		go func() {
			n, err := io.Copy(mw, resp)
			copyN <- n
			copyErr <- err
			_ = pw.Close()
		}()

		playErr := playToSpeakers(ctx, pr)
		copyNVal := <-copyN
		copyErrVal := <-copyErr
		if copyErrVal != nil {
			return copyNVal, copyErrVal
		}
		return copyNVal, playErr
	}

	if len(writers) == 0 {
		return 0, errors.New("nothing to do: enable --play or provide --output")
	}

	mw := io.MultiWriter(writers...)
	n, err := io.Copy(mw, resp)
	return n, err
}

func convertAndPlay(ctx context.Context, client *minimax.Client, opts speakOptions, payload minimax.TTSRequest) (int64, error) {
	data, err := client.ConvertTTS(ctx, payload)
	if err != nil {
		return 0, err
	}
	n := int64(len(data))

	if opts.outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.outputPath), 0o755); err != nil {
			return n, err
		}
		if err := os.WriteFile(opts.outputPath, data, 0o644); err != nil {
			return n, err
		}
	}

	if opts.play {
		pr, pw := io.Pipe()
		go func() {
			_, _ = pw.Write(data)
			_ = pw.Close()
		}()
		return n, playToSpeakers(ctx, pr)
	}
	if opts.outputPath == "" {
		return n, errors.New("nothing to do: enable --play or provide --output")
	}
	return n, nil
}

func resolveVoice(ctx context.Context, client *minimax.Client, voiceInput, category string, forceID bool) (string, error) {
	voiceInput = strings.TrimSpace(voiceInput)
	if voiceInput == "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx, category)
		if err != nil {
			return "", fmt.Errorf("voice not specified and failed to fetch voices: %w", err)
		}
		if len(voices) == 0 {
			return "", errors.New("no voices available; specify --voice or set MINIMAX_VOICE_ID")
		}
		fmt.Fprintf(os.Stderr, "defaulting to voice %s (%s)\n", voiceLabel(voices[0]), voices[0].VoiceID)
		return voices[0].VoiceID, nil
	}
	if voiceInput == "?" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx, category)
		if err != nil {
			return "", err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintf(w, "VOICE ID\tNAME\tCATEGORY\n"); err != nil {
			return "", err
		}
		for _, v := range voices {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", v.VoiceID, voiceLabel(v), v.Category); err != nil {
				return "", err
			}
		}
		if err := w.Flush(); err != nil {
			return "", err
		}
		return "", nil
	}

	if forceID {
		return voiceInput, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	voices, err := client.ListVoices(ctx, category)
	if err != nil {
		return "", err
	}
	voiceInputLower := strings.ToLower(voiceInput)

	// First, check for exact match (case-insensitive)
	for _, v := range voices {
		if strings.ToLower(v.VoiceID) == voiceInputLower {
			fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", voiceLabel(v), v.VoiceID)
			return v.VoiceID, nil
		}
		if strings.ToLower(v.VoiceName) == voiceInputLower {
			fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", voiceLabel(v), v.VoiceID)
			return v.VoiceID, nil
		}
	}

	// Then, check for substring match (case-insensitive)
	for _, v := range voices {
		if strings.Contains(strings.ToLower(voiceLabel(v)), voiceInputLower) || strings.Contains(strings.ToLower(v.VoiceID), voiceInputLower) {
			fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", voiceLabel(v), v.VoiceID)
			return v.VoiceID, nil
		}
	}

	return "", fmt.Errorf("voice %q not found; try 'sag voices' or -v '?'", voiceInput)
}

func voiceLabel(voice minimax.Voice) string {
	if strings.TrimSpace(voice.VoiceName) != "" {
		return voice.VoiceName
	}
	if len(voice.Description) > 0 {
		return strings.Join(voice.Description, ", ")
	}
	return voice.VoiceID
}

func inferFormatFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		return "mp3_44100_128"
	case ".wav", ".wave":
		return "wav"
	case ".flac":
		return "flac"
	case ".pcm":
		return "pcm"
	default:
		return ""
	}
}

func outputFormatKind(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return ""
	}
	if idx := strings.Index(lower, "_"); idx >= 0 {
		lower = lower[:idx]
	}
	switch lower {
	case "mp3", "wav", "flac", "pcm":
		return lower
	default:
		return ""
	}
}

func parseOutputFormat(value string) (string, *minimax.AudioSetting, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil, nil
	}
	lower := strings.ToLower(value)
	parts := strings.Split(lower, "_")
	format := parts[0]

	if !isSupportedFormat(format) {
		return lower, nil, nil
	}

	setting := &minimax.AudioSetting{Format: format}
	if len(parts) == 1 {
		return format, setting, nil
	}
	if len(parts) > 3 {
		return "", nil, fmt.Errorf("invalid format %q (expected format[_sampleRate[_bitrate]])", value)
	}

	if len(parts) >= 2 {
		sampleRate, err := strconv.Atoi(parts[1])
		if err != nil || sampleRate <= 0 {
			return "", nil, fmt.Errorf("invalid sample rate in format %q", value)
		}
		setting.SampleRate = &sampleRate
	}
	if len(parts) == 3 {
		bitrate, err := strconv.Atoi(parts[2])
		if err != nil || bitrate <= 0 {
			return "", nil, fmt.Errorf("invalid bitrate in format %q", value)
		}
		if bitrate <= 1000 {
			bitrate *= 1000
		}
		setting.Bitrate = &bitrate
	}
	return format, setting, nil
}

func isSupportedFormat(format string) bool {
	switch format {
	case "mp3", "wav", "flac", "pcm":
		return true
	default:
		return false
	}
}

func normalizeVoiceCategory(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "all":
		return "all", nil
	case "system", "voice_cloning", "voice_generation":
		return v, nil
	default:
		return "", fmt.Errorf("invalid voice category %q (expected system|voice_cloning|voice_generation|all)", value)
	}
}
