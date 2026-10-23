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
	"github.com/steipete/sag/internal/sixtydb"
	"github.com/steipete/sag/internal/tts"

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
	timeout     time.Duration
	player      string

	speakerBoost   bool
	noSpeakerBoost bool
}

const defaultWPM = 175 // matches macOS `say` default rate

var playToSpeakers = audio.StreamToSpeakers

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
		Short: "Speak the provided text using ElevenLabs TTS (default: stream to speakers)",
		Long:  "If no text argument is provided, the command reads from stdin.\n\nTip: run `sag prompting` for model-specific prompting tips and recommended flag combinations.",
		Args:  cobra.ArbitraryArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return ensureProviderConfigured()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := applyRateAndSpeed(&opts); err != nil {
				return err
			}
			if err := applyCompatibilityFlags(cmd, &opts); err != nil {
				return err
			}
			if err := applyTimeoutFromEnv(cmd, &opts); err != nil {
				return err
			}

			provider, err := selectProvider()
			if err != nil {
				return err
			}

			forceVoiceID := cmd.Flags().Changed("voice-id")
			voiceInput := opts.voiceID
			if provider.name == providerElevenLabs && voiceInput == "" {
				if env := os.Getenv("ELEVENLABS_VOICE_ID"); env != "" {
					voiceInput = env
					forceVoiceID = true
				} else if env := os.Getenv("SAG_VOICE_ID"); env != "" {
					voiceInput = env
					forceVoiceID = true
				}
			}

			voiceID, err := resolveVoice(cmd.Context(), provider.voices, voiceInput, forceVoiceID)
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

			// If user provided output path with a known extension, infer a compatible format
			// unless --format was explicitly set. Explicit format selection should always win.
			if opts.outputPath != "" {
				if !cmd.Flags().Changed("format") {
					if inferred := inferFormatFromExt(opts.outputPath); inferred != "" {
						opts.outputFmt = inferred
					}
				}
				// Disable playback when -o is set, unless --play was explicitly provided
				if !cmd.Flags().Changed("play") {
					opts.play = false
				}
			}

			if provider.name == providerSixtyDB {
				if err := prepareSixtyDBOptions(cmd, &opts); err != nil {
					return err
				}
			}

			ctx, cancel, err := ttsContext(cmd.Context(), opts.timeout)
			if err != nil {
				return err
			}
			defer cancel()

			var (
				streamFunc  func(context.Context) (io.ReadCloser, error)
				convertFunc func(context.Context) ([]byte, error)
			)
			switch provider.name {
			case providerElevenLabs:
				payload, err := buildElevenLabsTTSRequest(cmd, opts, text)
				if err != nil {
					return err
				}
				streamFunc = func(ctx context.Context) (io.ReadCloser, error) {
					return provider.elevenlabs.StreamTTS(ctx, opts.voiceID, payload, opts.latencyTier)
				}
				convertFunc = func(ctx context.Context) ([]byte, error) {
					return provider.elevenlabs.ConvertTTS(ctx, opts.voiceID, payload)
				}
			case providerSixtyDB:
				payload, err := buildSixtyDBTTSRequest(cmd, opts, text)
				if err != nil {
					return err
				}
				streamFunc = func(ctx context.Context) (io.ReadCloser, error) {
					return provider.sixtydb.StreamTTS(ctx, payload)
				}
				convertFunc = func(ctx context.Context) ([]byte, error) {
					return provider.sixtydb.ConvertTTS(ctx, payload)
				}
			default:
				return fmt.Errorf("unsupported provider %q", provider.name)
			}

			start := time.Now()
			var bytes int64
			if opts.stream {
				n, err := streamAndPlay(ctx, opts, streamFunc)
				bytes = n
				if err != nil {
					return err
				}
			} else {
				n, err := convertAndPlay(ctx, opts, convertFunc)
				bytes = n
				if err != nil {
					return err
				}
			}
			if opts.metrics {
				if provider.name == providerElevenLabs {
					fmt.Fprintf(os.Stderr, "metrics: chars=%d bytes=%d provider=%s model=%s voice=%s stream=%t latencyTier=%d dur=%s\n",
						len([]rune(text)), bytes, provider.name, opts.modelID, opts.voiceID, opts.stream, opts.latencyTier, time.Since(start).Truncate(time.Millisecond))
				} else {
					fmt.Fprintf(os.Stderr, "metrics: chars=%d bytes=%d provider=%s voice=%s stream=%t dur=%s\n",
						len([]rune(text)), bytes, provider.name, opts.voiceID, opts.stream, time.Since(start).Truncate(time.Millisecond))
				}
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
	cmd.Flags().Bool("no-stream", false, "Disable streaming and wait for the full audio response")
	cmd.Flags().BoolVar(&opts.play, "play", opts.play, "Play audio through speakers")
	cmd.Flags().Bool("no-play", false, "Disable speaker playback")
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
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "Maximum time for TTS generation (0 disables sag's internal timeout; SAG_TIMEOUT)")
	cmd.Flags().StringVar(&opts.player, "player", "auto", "Audio backend: auto (afplay on macOS, oto elsewhere), afplay, oto (SAG_PLAYER)")
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

func applyCompatibilityFlags(cmd *cobra.Command, opts *speakOptions) error {
	noPlay, err := cmd.Flags().GetBool("no-play")
	if err != nil {
		return err
	}
	if noPlay {
		if cmd.Flags().Changed("play") && opts.play {
			return errors.New("choose only one of --play or --no-play")
		}
		opts.play = false
	}

	noStream, err := cmd.Flags().GetBool("no-stream")
	if err != nil {
		return err
	}
	if noStream {
		if cmd.Flags().Changed("stream") && opts.stream {
			return errors.New("choose only one of --stream or --no-stream")
		}
		opts.stream = false
	}
	return nil
}

func playbackFunc(choice string) (func(context.Context, io.Reader) error, error) {
	choice = strings.ToLower(strings.TrimSpace(choice))
	if choice == "" || choice == "auto" {
		if env := strings.TrimSpace(os.Getenv("SAG_PLAYER")); env != "" {
			choice = strings.ToLower(env)
		}
	}
	switch choice {
	case "", "auto":
		return playToSpeakers, nil
	case "afplay":
		return audio.StreamViaAfplay, nil
	case "oto":
		return audio.StreamViaOto, nil
	default:
		return nil, fmt.Errorf("unknown player %q; choose auto, afplay, or oto", choice)
	}
}

func applyTimeoutFromEnv(cmd *cobra.Command, opts *speakOptions) error {
	if cmd.Flags().Changed("timeout") {
		return nil
	}
	env := strings.TrimSpace(os.Getenv("SAG_TIMEOUT"))
	if env == "" {
		return nil
	}
	timeout, err := time.ParseDuration(env)
	if err != nil {
		return fmt.Errorf("invalid SAG_TIMEOUT %q: %w", env, err)
	}
	opts.timeout = timeout
	return nil
}

func ttsContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	if timeout < 0 {
		return nil, nil, errors.New("timeout must be >= 0")
	}
	if timeout == 0 {
		ctx, cancel := context.WithCancel(parent)
		return ctx, cancel, nil
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, nil
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

func buildElevenLabsTTSRequest(cmd *cobra.Command, opts speakOptions, text string) (elevenlabs.TTSRequest, error) {
	flags := cmd.Flags()

	var stabilityPtr *float64
	if flags.Changed("stability") {
		if opts.stability < 0 || opts.stability > 1 {
			return elevenlabs.TTSRequest{}, errors.New("stability must be between 0 and 1")
		}
		// The discrete 0/0.5/1 constraint is specific to ElevenLabs eleven_v3.
		if opts.modelID == "eleven_v3" && !floatEqualsOneOf(opts.stability, []float64{0, 0.5, 1}) {
			return elevenlabs.TTSRequest{}, errors.New("for eleven_v3, stability must be one of 0.0, 0.5, 1.0 (Creative/Natural/Robust)")
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

func prepareSixtyDBOptions(cmd *cobra.Command, opts *speakOptions) error {
	unsupported := changedSixtyDBUnsupportedFlags(cmd.Flags().Changed)
	if len(unsupported) > 0 {
		return fmt.Errorf("60db does not support %s", strings.Join(unsupported, ", "))
	}

	if !cmd.Flags().Changed("format") {
		switch {
		case strings.EqualFold(filepath.Ext(opts.outputPath), ".flac"):
			opts.outputFmt = "flac"
		case opts.outputPath == "" || inferFormatFromExt(opts.outputPath) == "":
			opts.outputFmt = "wav"
		}
	}

	format := sixtydb.CanonicalOutputFormat(opts.outputFmt)
	if format == "" {
		format = "wav"
	}
	if format != "wav" {
		return fmt.Errorf("60db currently returns raw PCM and supports wav output only; got %s", format)
	}
	opts.outputFmt = format

	if opts.stream {
		if cmd.Flags().Changed("stream") {
			return errors.New("60db live synthesis does not support streaming; omit --stream")
		}
		opts.stream = false
	}
	return nil
}

func buildSixtyDBTTSRequest(cmd *cobra.Command, opts speakOptions, text string) (sixtydb.TTSRequest, error) {
	flags := cmd.Flags()
	speed := opts.speed
	req := sixtydb.TTSRequest{
		Text:         text,
		VoiceID:      opts.voiceID,
		Speed:        &speed,
		OutputFormat: opts.outputFmt,
		SampleRate:   sixtydb.DefaultSampleRate,
	}

	if flags.Changed("stability") {
		if opts.stability < 0 || opts.stability > 1 {
			return sixtydb.TTSRequest{}, errors.New("stability must be between 0 and 1")
		}
		stability := opts.stability * 100
		req.Stability = &stability
	}
	if flags.Changed("similarity") || flags.Changed("similarity-boost") {
		if opts.similarity < 0 || opts.similarity > 1 {
			return sixtydb.TTSRequest{}, errors.New("similarity must be between 0 and 1")
		}
		similarity := opts.similarity * 100
		req.Similarity = &similarity
	}
	return req, nil
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

func streamAndPlay(ctx context.Context, opts speakOptions, stream func(context.Context) (io.ReadCloser, error)) (int64, error) {
	if !opts.play && opts.outputPath == "" {
		return 0, errors.New("nothing to do: enable --play or provide --output")
	}

	resp, err := stream(ctx)
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
		player, err := playbackFunc(opts.player)
		if err != nil {
			return 0, err
		}
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

		playErr := player(ctx, pr)
		// Stop both a blocked pipe write and an upstream read before joining the copy.
		_ = pr.Close()
		_ = resp.Close()
		copyNVal := <-copyN
		copyErrVal := <-copyErr
		if playErr != nil {
			return copyNVal, playErr
		}
		if copyErrVal != nil && (opts.outputPath != "" || !errors.Is(copyErrVal, io.ErrClosedPipe)) {
			return copyNVal, copyErrVal
		}
		return copyNVal, nil
	}

	mw := io.MultiWriter(writers...)
	n, err := io.Copy(mw, resp)
	return n, err
}

func convertAndPlay(ctx context.Context, opts speakOptions, convert func(context.Context) ([]byte, error)) (int64, error) {
	if !opts.play && opts.outputPath == "" {
		return 0, errors.New("nothing to do: enable --play or provide --output")
	}

	data, err := convert(ctx)
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
		player, err := playbackFunc(opts.player)
		if err != nil {
			return n, err
		}
		pr, pw := io.Pipe()
		go func() {
			_, _ = pw.Write(data)
			_ = pw.Close()
		}()
		return n, player(ctx, pr)
	}
	return n, nil
}

func resolveVoice(ctx context.Context, client tts.VoiceCatalog, voiceInput string, forceID bool) (string, error) {
	voiceInput = strings.TrimSpace(voiceInput)
	if voiceInput == "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		voices, err := client.ListVoices(ctx)
		if err != nil {
			return "", fmt.Errorf("voice not specified and failed to fetch voices: %w", err)
		}
		if len(voices) == 0 {
			return "", errors.New("no voices available; specify --voice")
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
	case ".ogg", ".opus":
		return "opus_48000_64"
	default:
		return ""
	}
}
