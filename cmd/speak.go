package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/steipete/sag/internal/audio"
	"github.com/steipete/sag/internal/elevenlabs"
	"github.com/steipete/sag/internal/minimax"

	"github.com/spf13/cobra"
)

type speakOptions struct {
	voiceID     string
	modelID     string
	outputPath  string
	outputFmt   string
	stream      bool
	play        bool
	latencyTier int
	speed       float64
	rateWPM     int
	inputFile   string
	stability   float64
	similarity  float64
	style       float64
	seed        uint64
	normalize   string
	lang        string
	metrics     bool

	speakerBoost   bool
	noSpeakerBoost bool

	minimaxVolume                  float64
	minimaxPitch                   int
	minimaxEmotion                 string
	minimaxLanguage                string
	minimaxAccent                  string
	minimaxTone                    []string
	minimaxTextNormalization       bool
	minimaxLatexRead               bool
	minimaxContinuousSound         bool
	minimaxVoiceModifyPitch        int
	minimaxVoiceModifyIntensity    int
	minimaxVoiceModifyTimbre       int
	minimaxVoiceModifySoundEffects string
}

const defaultWPM = 175 // matches macOS `say` default rate

var playToSpeakers = audio.StreamToSpeakers

const (
	providerElevenLabs = "elevenlabs"
	providerMiniMax    = "minimax"
)

func init() {
	opts := speakOptions{
		modelID:   "eleven_v3",
		outputFmt: "mp3_44100_128",
		stream:    true,
		play:      true,
		speed:     1.0,
	}

	cmd := &cobra.Command{
		Use:   "speak [text]",
		Short: "Speak the provided text using TTS (default: stream to speakers)",
		Long:  "If no text argument is provided, the command reads from stdin.\n\nTip: run `sag prompting` for model-specific prompting tips and recommended flag combinations.",
		Args:  cobra.ArbitraryArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return ensureAPIKeyForProvider(detectProvider(opts.modelID))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := applyRateAndSpeed(&opts); err != nil {
				return err
			}

			provider := detectProvider(opts.modelID)
			forceVoiceID := cmd.Flags().Changed("voice-id")
			voiceInput := opts.voiceID
			if voiceInput == "" {
				if provider == providerMiniMax {
					if env := os.Getenv("MINIMAX_VOICE_ID"); env != "" {
						voiceInput = env
						forceVoiceID = true
					} else if env := os.Getenv("SAG_VOICE_ID"); env != "" {
						voiceInput = env
						forceVoiceID = true
					}
				} else {
					if env := os.Getenv("ELEVENLABS_VOICE_ID"); env != "" {
						voiceInput = env
						forceVoiceID = true
					} else if env := os.Getenv("SAG_VOICE_ID"); env != "" {
						voiceInput = env
						forceVoiceID = true
					}
				}
			}
			elevenClient := elevenlabs.NewClient(cfg.APIKey, cfg.BaseURL)
			miniClient := minimax.NewClient(cfg.APIKey, minimaxBaseURL())

			switch provider {
			case providerMiniMax:
				voiceID, err := resolveMiniMaxVoice(cmd.Context(), miniClient, voiceInput, forceVoiceID)
				if err != nil {
					return err
				}
				if voiceID == "" {
					return nil
				}
				opts.voiceID = voiceID
			default:
				voiceID, err := resolveVoice(cmd.Context(), elevenClient, voiceInput, forceVoiceID)
				if err != nil {
					return err
				}
				if voiceID == "" {
					// Likely printed voices for '?' request.
					return nil
				}
				opts.voiceID = voiceID
			}

			text, err := resolveText(args, opts.inputFile)
			if err != nil {
				return err
			}

			// If user provided output path with a known extension, infer a compatible format.
			if opts.outputPath != "" {
				if provider == providerMiniMax {
					if inferred := inferMiniMaxFormatFromExt(opts.outputPath); inferred != "" {
						opts.outputFmt = inferred
					}
				} else if inferred := inferFormatFromExt(opts.outputPath); inferred != "" {
					opts.outputFmt = inferred
				}
				// Disable playback when -o is set, unless --play was explicitly provided
				if !cmd.Flags().Changed("play") {
					opts.play = false
				}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 90*time.Second)
			defer cancel()

			start := time.Now()
			var bytes int64
			switch provider {
			case providerMiniMax:
				payload, err := buildMiniMaxTTSRequest(cmd, opts, text)
				if err != nil {
					return err
				}
				if opts.stream {
					n, err := streamAndPlayMiniMax(ctx, miniClient, opts, payload)
					bytes = n
					if err != nil {
						return err
					}
				} else {
					n, err := convertAndPlayMiniMax(ctx, miniClient, opts, payload)
					bytes = n
					if err != nil {
						return err
					}
				}
			default:
				payload, err := buildTTSRequest(cmd, opts, text)
				if err != nil {
					return err
				}
				if opts.stream {
					n, err := streamAndPlay(ctx, elevenClient, opts, payload)
					bytes = n
					if err != nil {
						return err
					}
				} else {
					n, err := convertAndPlay(ctx, elevenClient, opts, payload)
					bytes = n
					if err != nil {
						return err
					}
				}
			}
			if opts.metrics {
				fmt.Fprintf(os.Stderr, "metrics: chars=%d bytes=%d model=%s voice=%s stream=%t latencyTier=%d dur=%s\n",
					len([]rune(text)), bytes, opts.modelID, opts.voiceID, opts.stream, opts.latencyTier, time.Since(start).Truncate(time.Millisecond))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.voiceID, "voice-id", "", "Voice ID to use (ELEVENLABS_VOICE_ID)")
	cmd.Flags().StringVarP(&opts.voiceID, "voice", "v", "", "Alias for --voice-id; accepts name or ID; use '?' to list voices")
	cmd.Flags().StringVar(&opts.modelID, "model-id", opts.modelID, "Model ID (default: eleven_v3). Common: eleven_multilingual_v2 (stable), eleven_flash_v2_5 (fast/cheap), eleven_turbo_v2_5 (balanced).")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "Write audio to file (disables playback unless --play is also set)")
	cmd.Flags().StringVar(&opts.outputFmt, "format", opts.outputFmt, "Output format (e.g. mp3_44100_128)")
	cmd.Flags().BoolVar(&opts.stream, "stream", opts.stream, "Stream audio while generating")
	cmd.Flags().BoolVar(&opts.play, "play", opts.play, "Play audio through speakers")
	cmd.Flags().IntVar(&opts.latencyTier, "latency-tier", 0, "Streaming latency tier (0=default,1-4 lower latency may cost more)")
	cmd.Flags().Float64Var(&opts.speed, "speed", opts.speed, "Speech speed multiplier (e.g. 1.1 faster, 0.9 slower)")
	cmd.Flags().IntVarP(&opts.rateWPM, "rate", "r", 0, "macOS say-style words-per-minute; overrides --speed when set (default 175 wpm)")
	cmd.Flags().Float64Var(&opts.stability, "stability", 0, "Voice stability (0..1; higher = more consistent, less expressive)")
	cmd.Flags().Float64Var(&opts.similarity, "similarity", 0, "Voice similarity boost (0..1; higher = closer to reference voice)")
	cmd.Flags().Float64Var(&opts.similarity, "similarity-boost", 0, "Alias for --similarity")
	cmd.Flags().Float64Var(&opts.style, "style", 0, "Voice style exaggeration (0..1; higher = more stylized delivery)")
	cmd.Flags().BoolVar(&opts.speakerBoost, "speaker-boost", false, "Enable speaker boost (can improve clarity; model dependent)")
	cmd.Flags().BoolVar(&opts.noSpeakerBoost, "no-speaker-boost", false, "Disable speaker boost")
	cmd.Flags().Uint64Var(&opts.seed, "seed", 0, "Best-effort deterministic seed (0..4294967295; helps repeatability across runs)")
	cmd.Flags().StringVar(&opts.normalize, "normalize", "", "Text normalization: auto|on|off (numbers/units/URLs; when set)")
	cmd.Flags().StringVar(&opts.lang, "lang", "", "Language code (2-letter ISO 639-1; influences normalization; when set)")
	cmd.Flags().BoolVar(&opts.metrics, "metrics", false, "Print request metrics to stderr (chars, bytes, duration, etc.)")
	cmd.Flags().StringVarP(&opts.inputFile, "input-file", "f", "", "Read text from file (use '-' for stdin), matching macOS say -f")
	cmd.Flags().Float64Var(&opts.minimaxVolume, "volume", 0, "MiniMax voice volume (0..10; when set)")
	cmd.Flags().IntVar(&opts.minimaxPitch, "pitch", 0, "MiniMax voice pitch (-12..12; when set)")
	cmd.Flags().StringVar(&opts.minimaxEmotion, "emotion", "", "MiniMax voice emotion (model dependent)")
	cmd.Flags().StringVar(&opts.minimaxLanguage, "language", "", "MiniMax language boost (e.g. English, Chinese,Yue; when set)")
	cmd.Flags().StringVar(&opts.minimaxAccent, "accent", "", "Alias for --language (MiniMax language boost)")
	cmd.Flags().StringArrayVar(&opts.minimaxTone, "tone", nil, "MiniMax pronunciation tone override (repeatable, e.g. \"omg/oh my god\")")
	cmd.Flags().BoolVar(&opts.minimaxTextNormalization, "text-normalization", false, "MiniMax text normalization (improves digit reading; when set)")
	cmd.Flags().BoolVar(&opts.minimaxLatexRead, "latex-read", false, "MiniMax LaTeX formula reading (Chinese only; when set)")
	cmd.Flags().BoolVar(&opts.minimaxContinuousSound, "continuous-sound", false, "MiniMax continuous sound for smoother transitions (when set)")
	cmd.Flags().IntVar(&opts.minimaxVoiceModifyPitch, "voice-modify-pitch", 0, "MiniMax voice modify pitch (-100..100; when set)")
	cmd.Flags().IntVar(&opts.minimaxVoiceModifyIntensity, "voice-modify-intensity", 0, "MiniMax voice modify intensity (-100..100; when set)")
	cmd.Flags().IntVar(&opts.minimaxVoiceModifyTimbre, "voice-modify-timbre", 0, "MiniMax voice modify timbre (-100..100; when set)")
	cmd.Flags().StringVar(&opts.minimaxVoiceModifySoundEffects, "voice-modify-sound-effects", "", "MiniMax voice modify sound effects (e.g. spacious_echo, auditorium_echo, lofi_telephone, robotic)")
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
		// Map macOS `say` rate (words per minute) to ElevenLabs speed multiplier.
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

func buildTTSRequest(cmd *cobra.Command, opts speakOptions, text string) (elevenlabs.TTSRequest, error) {
	flags := cmd.Flags()

	var stabilityPtr *float64
	if flags.Changed("stability") {
		if opts.stability < 0 || opts.stability > 1 {
			return elevenlabs.TTSRequest{}, errors.New("stability must be between 0 and 1")
		}
		if opts.modelID == "eleven_v3" {
			if !floatEqualsOneOf(opts.stability, []float64{0, 0.5, 1}) {
				return elevenlabs.TTSRequest{}, errors.New("for eleven_v3, stability must be one of 0.0, 0.5, 1.0 (Creative/Natural/Robust)")
			}
		}
		stabilityPtr = &opts.stability
	}

	var similarityPtr *float64
	if flags.Changed("similarity") || flags.Changed("similarity-boost") {
		if opts.similarity < 0 || opts.similarity > 1 {
			return elevenlabs.TTSRequest{}, errors.New("similarity must be between 0 and 1")
		}
		similarityPtr = &opts.similarity
	}

	var stylePtr *float64
	if flags.Changed("style") {
		if opts.style < 0 || opts.style > 1 {
			return elevenlabs.TTSRequest{}, errors.New("style must be between 0 and 1")
		}
		stylePtr = &opts.style
	}

	if flags.Changed("speaker-boost") && flags.Changed("no-speaker-boost") {
		return elevenlabs.TTSRequest{}, errors.New("choose only one of --speaker-boost or --no-speaker-boost")
	}
	var speakerBoostPtr *bool
	if flags.Changed("speaker-boost") {
		v := true
		speakerBoostPtr = &v
	} else if flags.Changed("no-speaker-boost") {
		v := false
		speakerBoostPtr = &v
	}

	var seedPtr *uint32
	if flags.Changed("seed") {
		if opts.seed > 4294967295 {
			return elevenlabs.TTSRequest{}, errors.New("seed must be between 0 and 4294967295")
		}
		v := uint32(opts.seed)
		seedPtr = &v
	}

	normalize := strings.ToLower(strings.TrimSpace(opts.normalize))
	if flags.Changed("normalize") {
		switch normalize {
		case "auto", "on", "off":
		default:
			return elevenlabs.TTSRequest{}, errors.New("normalize must be one of: auto, on, off")
		}
	} else {
		normalize = ""
	}

	lang := strings.ToLower(strings.TrimSpace(opts.lang))
	if flags.Changed("lang") {
		if len(lang) != 2 {
			return elevenlabs.TTSRequest{}, errors.New("lang must be a 2-letter ISO 639-1 code (e.g. en, de, fr)")
		}
		for _, r := range lang {
			if r < 'a' || r > 'z' {
				return elevenlabs.TTSRequest{}, errors.New("lang must be a 2-letter ISO 639-1 code (e.g. en, de, fr)")
			}
		}
	} else {
		lang = ""
	}

	speed := opts.speed
	return elevenlabs.TTSRequest{
		Text:                   text,
		ModelID:                opts.modelID,
		OutputFormat:           opts.outputFmt,
		Seed:                   seedPtr,
		ApplyTextNormalization: normalize,
		LanguageCode:           lang,
		VoiceSettings: &elevenlabs.VoiceSettings{
			Speed:           &speed,
			Stability:       stabilityPtr,
			SimilarityBoost: similarityPtr,
			Style:           stylePtr,
			UseSpeakerBoost: speakerBoostPtr,
		},
	}, nil
}

func floatEqualsOneOf(v float64, allowed []float64) bool {
	const eps = 1e-9
	for _, a := range allowed {
		d := v - a
		if d < 0 {
			d = -d
		}
		if d <= eps {
			return true
		}
	}
	return false
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

func streamAndPlay(ctx context.Context, client *elevenlabs.Client, opts speakOptions, payload elevenlabs.TTSRequest) (int64, error) {
	resp, err := client.StreamTTS(ctx, opts.voiceID, payload, opts.latencyTier)
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

func convertAndPlay(ctx context.Context, client *elevenlabs.Client, opts speakOptions, payload elevenlabs.TTSRequest) (int64, error) {
	data, err := client.ConvertTTS(ctx, opts.voiceID, payload)
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

func streamAndPlayMiniMax(ctx context.Context, client *minimax.Client, opts speakOptions, payload minimax.TTSRequest) (int64, error) {
	resp, err := client.StreamTTS(ctx, opts.voiceID, payload)
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

func convertAndPlayMiniMax(ctx context.Context, client *minimax.Client, opts speakOptions, payload minimax.TTSRequest) (int64, error) {
	data, err := client.ConvertTTS(ctx, opts.voiceID, payload)
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

func resolveVoice(ctx context.Context, client *elevenlabs.Client, voiceInput string, forceID bool) (string, error) {
	voiceInput = strings.TrimSpace(voiceInput)
	if voiceInput == "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx)
		if err != nil {
			return "", fmt.Errorf("voice not specified and failed to fetch voices: %w", err)
		}
		if len(voices) == 0 {
			return "", errors.New("no voices available; specify --voice or set ELEVENLABS_VOICE_ID")
		}
		fmt.Fprintf(os.Stderr, "defaulting to voice %s (%s)\n", voices[0].Name, voices[0].VoiceID)
		return voices[0].VoiceID, nil
	}
	if voiceInput == "?" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx)
		if err != nil {
			return "", err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintf(w, "VOICE ID\tNAME\tCATEGORY\tDESCRIPTION\n"); err != nil {
			return "", err
		}
		for _, v := range voices {
			desc := strings.ReplaceAll(v.Description, "\t", " ")
			desc = strings.ReplaceAll(desc, "\n", " ")
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", v.VoiceID, v.Name, v.Category, desc); err != nil {
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

	if looksLikeVoiceID(voiceInput) {
		if containsDigit(voiceInput) {
			return voiceInput, nil
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx)
		if err != nil {
			return "", err
		}
		voiceInputLower := strings.ToLower(voiceInput)
		for _, v := range voices {
			if strings.ToLower(v.Name) == voiceInputLower {
				fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", v.Name, v.VoiceID)
				return v.VoiceID, nil
			}
		}
		return voiceInput, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	voices, err := client.ListVoices(ctx)
	if err != nil {
		return "", err
	}
	voiceInputLower := strings.ToLower(voiceInput)

	// First, check for exact match (case-insensitive)
	for _, v := range voices {
		if strings.ToLower(v.Name) == voiceInputLower {
			fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", v.Name, v.VoiceID)
			return v.VoiceID, nil
		}
	}

	// Then, check for substring match (case-insensitive)
	for _, v := range voices {
		if strings.Contains(strings.ToLower(v.Name), voiceInputLower) {
			fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", v.Name, v.VoiceID)
			return v.VoiceID, nil
		}
	}

	return "", fmt.Errorf("voice %q not found; try 'sag voices' or -v '?'", voiceInput)
}

func resolveMiniMaxVoice(ctx context.Context, client *minimax.Client, voiceInput string, forceID bool) (string, error) {
	voiceInput = strings.TrimSpace(voiceInput)
	if voiceInput == "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx)
		if err != nil {
			return "", fmt.Errorf("voice not specified and failed to fetch voices: %w", err)
		}
		if len(voices) == 0 {
			return "", errors.New("no voices available; specify --voice or set MINIMAX_VOICE_ID")
		}
		fmt.Fprintf(os.Stderr, "defaulting to voice %s (%s)\n", voices[0].Name, voices[0].VoiceID)
		return voices[0].VoiceID, nil
	}
	if voiceInput == "?" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx)
		if err != nil {
			return "", err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintf(w, "VOICE ID\tNAME\tCATEGORY\n"); err != nil {
			return "", err
		}
		for _, v := range voices {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", v.VoiceID, v.Name, v.Category); err != nil {
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
	voices, err := client.ListVoices(ctx)
	if err != nil {
		return voiceInput, nil
	}
	voiceInputLower := strings.ToLower(voiceInput)
	for _, v := range voices {
		if strings.ToLower(v.VoiceID) == voiceInputLower || strings.ToLower(v.Name) == voiceInputLower {
			fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", v.Name, v.VoiceID)
			return v.VoiceID, nil
		}
	}
	for _, v := range voices {
		if strings.Contains(strings.ToLower(v.Name), voiceInputLower) {
			fmt.Fprintf(os.Stderr, "using voice %s (%s)\n", v.Name, v.VoiceID)
			return v.VoiceID, nil
		}
	}
	return voiceInput, nil
}

func looksLikeVoiceID(voiceInput string) bool {
	return len(voiceInput) >= 15 && !strings.ContainsRune(voiceInput, ' ')
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func inferFormatFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		return "mp3_44100_128"
	case ".wav", ".wave":
		return "pcm_44100"
	default:
		return ""
	}
}

func inferMiniMaxFormatFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		return "mp3"
	case ".wav", ".wave":
		return "wav"
	case ".flac":
		return "flac"
	default:
		return ""
	}
}

func detectProvider(modelID string) string {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	if strings.HasPrefix(modelID, "speech-") {
		return providerMiniMax
	}
	return providerElevenLabs
}

func minimaxBaseURL() string {
	host := strings.TrimSpace(os.Getenv("MINIMAX_API_HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("MINIMAX_BASE_URL"))
	}
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}
	return "https://" + host
}

func buildMiniMaxTTSRequest(cmd *cobra.Command, opts speakOptions, text string) (minimax.TTSRequest, error) {
	flags := cmd.Flags()

	format, err := normalizeMiniMaxFormat(opts.outputFmt)
	if err != nil {
		return minimax.TTSRequest{}, err
	}
	formatExplicit := flags.Changed("format") || opts.outputPath != ""
	if formatExplicit {
		if opts.stream && format != "mp3" {
			return minimax.TTSRequest{}, errors.New("MiniMax streaming supports mp3 only; use --no-stream for wav/flac")
		}
		if opts.play && format != "mp3" {
			return minimax.TTSRequest{}, errors.New("MiniMax playback supports mp3 only; use --output without --play for wav/flac")
		}
	} else {
		format = ""
	}

	var speedPtr *float64
	if flags.Changed("speed") || flags.Changed("rate") {
		speed := opts.speed
		speedPtr = &speed
	}

	var volumePtr *float64
	if flags.Changed("volume") {
		if opts.minimaxVolume <= 0 || opts.minimaxVolume > 10 {
			return minimax.TTSRequest{}, errors.New("volume must be between 0 and 10 (exclusive 0)")
		}
		volume := opts.minimaxVolume
		volumePtr = &volume
	}

	var pitchPtr *int
	if flags.Changed("pitch") {
		if opts.minimaxPitch < -12 || opts.minimaxPitch > 12 {
			return minimax.TTSRequest{}, errors.New("pitch must be between -12 and 12")
		}
		pitch := opts.minimaxPitch
		pitchPtr = &pitch
	}

	emotion := strings.TrimSpace(opts.minimaxEmotion)
	if flags.Changed("emotion") && emotion == "" {
		return minimax.TTSRequest{}, errors.New("emotion cannot be empty")
	}

	var textNormPtr *bool
	if flags.Changed("text-normalization") {
		v := opts.minimaxTextNormalization
		textNormPtr = &v
	}

	var latexReadPtr *bool
	if flags.Changed("latex-read") {
		v := opts.minimaxLatexRead
		latexReadPtr = &v
	}

	var continuousSoundPtr *bool
	if flags.Changed("continuous-sound") {
		v := opts.minimaxContinuousSound
		continuousSoundPtr = &v
	}

	var languageBoost string
	if flags.Changed("language") || flags.Changed("accent") {
		lang := strings.TrimSpace(opts.minimaxLanguage)
		accent := strings.TrimSpace(opts.minimaxAccent)
		if lang != "" && accent != "" && lang != accent {
			return minimax.TTSRequest{}, errors.New("choose only one of --language or --accent (or set the same value)")
		}
		if lang != "" {
			languageBoost = lang
		} else {
			languageBoost = accent
		}
		if languageBoost == "" {
			return minimax.TTSRequest{}, errors.New("language/accent cannot be empty")
		}
	}

	var tone []string
	if flags.Changed("tone") {
		for _, entry := range opts.minimaxTone {
			value := strings.TrimSpace(entry)
			if value == "" {
				return minimax.TTSRequest{}, errors.New("tone entries cannot be empty")
			}
			tone = append(tone, value)
		}
	}

	var voiceModify *minimax.VoiceModify
	var voiceModifyPitch *int
	var voiceModifyIntensity *int
	var voiceModifyTimbre *int
	var voiceModifySoundEffects *string
	if flags.Changed("voice-modify-pitch") {
		if opts.minimaxVoiceModifyPitch < -100 || opts.minimaxVoiceModifyPitch > 100 {
			return minimax.TTSRequest{}, errors.New("voice-modify-pitch must be between -100 and 100")
		}
		v := opts.minimaxVoiceModifyPitch
		voiceModifyPitch = &v
	}
	if flags.Changed("voice-modify-intensity") {
		if opts.minimaxVoiceModifyIntensity < -100 || opts.minimaxVoiceModifyIntensity > 100 {
			return minimax.TTSRequest{}, errors.New("voice-modify-intensity must be between -100 and 100")
		}
		v := opts.minimaxVoiceModifyIntensity
		voiceModifyIntensity = &v
	}
	if flags.Changed("voice-modify-timbre") {
		if opts.minimaxVoiceModifyTimbre < -100 || opts.minimaxVoiceModifyTimbre > 100 {
			return minimax.TTSRequest{}, errors.New("voice-modify-timbre must be between -100 and 100")
		}
		v := opts.minimaxVoiceModifyTimbre
		voiceModifyTimbre = &v
	}
	if flags.Changed("voice-modify-sound-effects") {
		value := strings.TrimSpace(opts.minimaxVoiceModifySoundEffects)
		if value == "" {
			return minimax.TTSRequest{}, errors.New("voice-modify-sound-effects cannot be empty")
		}
		voiceModifySoundEffects = &value
	}
	if voiceModifyPitch != nil || voiceModifyIntensity != nil || voiceModifyTimbre != nil || voiceModifySoundEffects != nil {
		voiceModify = &minimax.VoiceModify{
			Pitch:        voiceModifyPitch,
			Intensity:    voiceModifyIntensity,
			Timbre:       voiceModifyTimbre,
			SoundEffects: voiceModifySoundEffects,
		}
	}

	var pronunciationDict *minimax.PronunciationDict
	if len(tone) > 0 {
		pronunciationDict = &minimax.PronunciationDict{Tone: tone}
	}

	return minimax.TTSRequest{
		Model:             opts.modelID,
		Text:              text,
		Speed:             speedPtr,
		Volume:            volumePtr,
		Pitch:             pitchPtr,
		Emotion:           emotion,
		TextNormalization: textNormPtr,
		LatexRead:         latexReadPtr,
		AudioFormat:       format,
		LanguageBoost:     languageBoost,
		ContinuousSound:   continuousSoundPtr,
		PronunciationDict: pronunciationDict,
		VoiceModify:       voiceModify,
	}, nil
}

func normalizeMiniMaxFormat(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "", "mp3", "wav", "flac":
		if format == "" {
			return "mp3", nil
		}
		return format, nil
	case "mp3_44100_128":
		return "mp3", nil
	case "pcm_44100":
		return "wav", nil
	default:
		if strings.HasPrefix(format, "mp3_") {
			return "mp3", nil
		}
		if strings.HasPrefix(format, "pcm_") {
			return "wav", nil
		}
		return "", fmt.Errorf("format %q not supported for MiniMax (use mp3, wav, flac)", format)
	}
}
