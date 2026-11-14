package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func newSpeakTestCommand(t *testing.T) (*cobra.Command, *speakOptions) {
	t.Helper()
	opts := &speakOptions{
		modelID:   "eleven_multilingual_v2",
		outputFmt: "mp3_44100_128",
		speed:     1.0,
		stream:    true,
		play:      true,
	}
	cmd := &cobra.Command{Use: "speak"}
	cmd.Flags().StringVar(&opts.modelID, "model-id", opts.modelID, "")
	cmd.Flags().StringVar(&opts.outputFmt, "format", opts.outputFmt, "")
	cmd.Flags().BoolVar(&opts.stream, "stream", opts.stream, "")
	cmd.Flags().Float64Var(&opts.stability, "stability", 0, "")
	cmd.Flags().Float64Var(&opts.similarity, "similarity", 0, "")
	cmd.Flags().Float64Var(&opts.similarity, "similarity-boost", 0, "")
	cmd.Flags().Float64Var(&opts.style, "style", 0, "")
	cmd.Flags().BoolVar(&opts.speakerBoost, "speaker-boost", false, "")
	cmd.Flags().BoolVar(&opts.noSpeakerBoost, "no-speaker-boost", false, "")
	cmd.Flags().Uint64Var(&opts.seed, "seed", 0, "")
	cmd.Flags().StringVar(&opts.normalize, "normalize", "", "")
	cmd.Flags().StringVar(&opts.lang, "lang", "", "")
	cmd.Flags().IntVar(&opts.latencyTier, "latency-tier", 0, "")
	return cmd, opts
}

func TestBuildElevenLabsTTSRequest_DefaultsOmitOptionalFields(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)

	req, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildElevenLabsTTSRequest error: %v", err)
	}

	if req.Seed != nil {
		t.Fatalf("expected seed to be nil")
	}
	if req.ApplyTextNormalization != "" {
		t.Fatalf("expected apply_text_normalization to be empty, got %q", req.ApplyTextNormalization)
	}
	if req.LanguageCode != "" {
		t.Fatalf("expected language_code to be empty, got %q", req.LanguageCode)
	}
	if req.VoiceSettings == nil || req.VoiceSettings.Speed == nil {
		t.Fatalf("expected voice_settings.speed to be set")
	}
	if req.VoiceSettings.Stability != nil || req.VoiceSettings.SimilarityBoost != nil || req.VoiceSettings.Style != nil || req.VoiceSettings.UseSpeakerBoost != nil {
		t.Fatalf("expected optional voice settings to be nil")
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "stability") || strings.Contains(s, "similarity_boost") || strings.Contains(s, "style") || strings.Contains(s, "use_speaker_boost") {
		t.Fatalf("expected optional fields to be omitted, got %s", s)
	}
}

func TestBuildElevenLabsTTSRequest_SimilarityBoostAlias(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--similarity-boost", "0.9"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildElevenLabsTTSRequest error: %v", err)
	}
	if req.VoiceSettings.SimilarityBoost == nil || *req.VoiceSettings.SimilarityBoost != 0.9 {
		t.Fatalf("expected similarity_boost 0.9, got %#v", req.VoiceSettings.SimilarityBoost)
	}
}

func TestBuildElevenLabsTTSRequest_SpeakerBoostSetsJSONKey(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--speaker-boost"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildElevenLabsTTSRequest error: %v", err)
	}
	if req.VoiceSettings.UseSpeakerBoost == nil || *req.VoiceSettings.UseSpeakerBoost != true {
		t.Fatalf("expected use_speaker_boost true, got %#v", req.VoiceSettings.UseSpeakerBoost)
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), "use_speaker_boost") {
		t.Fatalf("expected JSON to contain use_speaker_boost, got %s", string(b))
	}
}

func TestElevenLabsStreamingPlayer(t *testing.T) {
	for _, tc := range []struct {
		name, format, player, env, want string
		play, stream                    bool
	}{
		{"streamed MP3", "mp3_44100_128", "auto", "", "oto", true, true},
		{"default choice", "mp3_44100_128", "", "", "oto", true, true},
		{"explicit afplay", "mp3_44100_128", "afplay", "", "afplay", true, true},
		{"environment afplay", "mp3_44100_128", "auto", "afplay", "auto", true, true},
		{"file only", "mp3_44100_128", "auto", "", "auto", false, true},
		{"buffered MP3", "mp3_44100_128", "auto", "", "auto", true, false},
		{"Opus", "opus_48000_64", "auto", "", "auto", true, true},
		{"PCM", "pcm_44100", "auto", "", "auto", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SAG_PLAYER", tc.env)
			opts := speakOptions{outputFmt: tc.format, player: tc.player, play: tc.play, stream: tc.stream}
			prepareElevenLabsPlayback(&opts)
			if opts.player != tc.want {
				t.Fatalf("player=%q, want %q", opts.player, tc.want)
			}
		})
	}
}

func TestBuildElevenLabsTTSRequest_InvalidNormalize(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--normalize", "wat"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "normalize must be one of") {
		t.Fatalf("expected normalize error, got %v", err)
	}
}

func TestBuildElevenLabsTTSRequest_InvalidLang(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--lang", "eng"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "lang must be a 2-letter") {
		t.Fatalf("expected lang error, got %v", err)
	}
}

func TestBuildElevenLabsTTSRequest_InvalidSeed(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--seed", "4294967296"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "seed must be between") {
		t.Fatalf("expected seed error, got %v", err)
	}
}

func TestBuildElevenLabsTTSRequest_SpeakerBoostConflict(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--speaker-boost", "--no-speaker-boost"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "choose only one") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestBuildElevenLabsTTSRequest_V3StabilityPresetsOnly(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	opts.modelID = "eleven_v3"
	if err := cmd.Flags().Parse([]string{"--stability", "0.55"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildElevenLabsTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "for eleven_v3, stability must be one of") {
		t.Fatalf("expected v3 stability preset error, got %v", err)
	}
}

func TestPrepareSixtyDBOptionsRejectsUnsupportedFlags(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--model-id", "foo", "--style", "0.3", "--latency-tier", "2"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	err := prepareSixtyDBOptions(cmd, opts)
	if err == nil {
		t.Fatal("expected unsupported flag error")
	}
	for _, flag := range []string{"--model-id", "--style", "--latency-tier"} {
		if !strings.Contains(err.Error(), flag) {
			t.Fatalf("expected %s in error, got %v", flag, err)
		}
	}
}

func TestPrepareSixtyDBOptionsDefaultsToWAVAndDisablesStreaming(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)

	if err := prepareSixtyDBOptions(cmd, opts); err != nil {
		t.Fatalf("prepareSixtyDBOptions error: %v", err)
	}
	if opts.outputFmt != "wav" {
		t.Fatalf("expected canonical wav format, got %q", opts.outputFmt)
	}
	if opts.stream {
		t.Fatalf("expected stream to be disabled for non-mp3 output")
	}
}

func TestPrepareSixtyDBOptionsAllowsWAVPlayback(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	opts.outputFmt = "wav"

	if err := prepareSixtyDBOptions(cmd, opts); err != nil {
		t.Fatalf("prepareSixtyDBOptions error: %v", err)
	}
	if !opts.play {
		t.Fatal("expected WAV playback to remain enabled")
	}
	if opts.stream {
		t.Fatal("expected stream to be disabled")
	}
}

func TestPrepareSixtyDBOptionsRejectsNonWAVOutput(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	opts.outputPath = "voice.FLAC"
	opts.play = false

	err := prepareSixtyDBOptions(cmd, opts)
	if err == nil || !strings.Contains(err.Error(), "supports wav output only") {
		t.Fatalf("expected non-WAV rejection, got %v", err)
	}
}

func TestPrepareSixtyDBOptionsRejectsExplicitStream(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--stream", "--format", "wav"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	err := prepareSixtyDBOptions(cmd, opts)
	if err == nil || !strings.Contains(err.Error(), "does not support streaming") {
		t.Fatalf("expected explicit stream rejection, got %v", err)
	}
}

func TestBuildSixtyDBTTSRequestScalesDocumentedFields(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	opts.voiceID = "voice_123"
	opts.stream = false
	opts.outputFmt = "wav"
	if err := cmd.Flags().Parse([]string{"--stability", "0.5", "--similarity-boost", "0.8"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildSixtyDBTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildSixtyDBTTSRequest error: %v", err)
	}
	if req.Text != "hello" || req.VoiceID != "voice_123" {
		t.Fatalf("unexpected request identity: %+v", req)
	}
	if req.Speed == nil || *req.Speed != 1.0 {
		t.Fatalf("expected speed 1.0, got %#v", req.Speed)
	}
	if req.Stability == nil || *req.Stability != 50 {
		t.Fatalf("expected stability 50, got %#v", req.Stability)
	}
	if req.Similarity == nil || *req.Similarity != 80 {
		t.Fatalf("expected similarity 80, got %#v", req.Similarity)
	}
	if req.OutputFormat != "wav" {
		t.Fatalf("expected selected output format, got %q", req.OutputFormat)
	}
	if req.SampleRate != 48000 {
		t.Fatalf("expected sample rate 48000, got %d", req.SampleRate)
	}
}

func TestBuildSixtyDBTTSRequestAlwaysIncludesOutputFormat(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	opts.outputFmt = "wav"
	req, err := buildSixtyDBTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildSixtyDBTTSRequest error: %v", err)
	}
	if req.OutputFormat != "wav" {
		t.Fatalf("expected output format, got %q", req.OutputFormat)
	}
}

func TestApplyCompatibilityFlagsNoPlayNoStream(t *testing.T) {
	opts := &speakOptions{play: true, stream: true}
	cmd := &cobra.Command{Use: "speak"}
	cmd.Flags().BoolVar(&opts.play, "play", opts.play, "")
	cmd.Flags().Bool("no-play", false, "")
	cmd.Flags().BoolVar(&opts.stream, "stream", opts.stream, "")
	cmd.Flags().Bool("no-stream", false, "")

	if err := cmd.Flags().Parse([]string{"--no-play", "--no-stream"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if err := applyCompatibilityFlags(cmd, opts); err != nil {
		t.Fatalf("applyCompatibilityFlags error: %v", err)
	}
	if opts.play || opts.stream {
		t.Fatalf("expected play=false and stream=false, got play=%t stream=%t", opts.play, opts.stream)
	}
}

func TestApplyCompatibilityFlagsConflict(t *testing.T) {
	opts := &speakOptions{play: true}
	cmd := &cobra.Command{Use: "speak"}
	cmd.Flags().BoolVar(&opts.play, "play", opts.play, "")
	cmd.Flags().Bool("no-play", false, "")
	cmd.Flags().BoolVar(&opts.stream, "stream", true, "")
	cmd.Flags().Bool("no-stream", false, "")

	if err := cmd.Flags().Parse([]string{"--play", "--no-play"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if err := applyCompatibilityFlags(cmd, opts); err == nil || !strings.Contains(err.Error(), "choose only one") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestPlaybackFuncExplicitBackends(t *testing.T) {
	if fn, err := playbackFunc("oto"); err != nil || fn == nil {
		t.Fatalf("playbackFunc oto error: %v", err)
	}
	if fn, err := playbackFunc("afplay"); err != nil || fn == nil {
		t.Fatalf("playbackFunc afplay error: %v", err)
	}
}

func TestPlaybackFuncEnv(t *testing.T) {
	t.Setenv("SAG_PLAYER", "oto")
	if fn, err := playbackFunc("auto"); err != nil || fn == nil {
		t.Fatalf("playbackFunc auto with env error: %v", err)
	}
}

func TestPlaybackFuncInvalid(t *testing.T) {
	_, err := playbackFunc("wat")
	if err == nil || !strings.Contains(err.Error(), "unknown player") {
		t.Fatalf("expected player error, got %v", err)
	}
}

func TestApplyTimeoutFromEnv(t *testing.T) {
	t.Setenv("SAG_TIMEOUT", "2m")
	opts := &speakOptions{}
	cmd := &cobra.Command{Use: "speak"}
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "")

	if err := applyTimeoutFromEnv(cmd, opts); err != nil {
		t.Fatalf("applyTimeoutFromEnv error: %v", err)
	}
	if opts.timeout != 2*time.Minute {
		t.Fatalf("timeout = %s, want 2m", opts.timeout)
	}
}

func TestApplyTimeoutFromEnvFlagWins(t *testing.T) {
	t.Setenv("SAG_TIMEOUT", "2m")
	opts := &speakOptions{}
	cmd := &cobra.Command{Use: "speak"}
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "")
	if err := cmd.Flags().Parse([]string{"--timeout", "30s"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	if err := applyTimeoutFromEnv(cmd, opts); err != nil {
		t.Fatalf("applyTimeoutFromEnv error: %v", err)
	}
	if opts.timeout != 30*time.Second {
		t.Fatalf("timeout = %s, want 30s", opts.timeout)
	}
}

func TestTTSContextNoDeadlineByDefault(t *testing.T) {
	ctx, cancel, err := ttsContext(context.Background(), 0)
	if err != nil {
		t.Fatalf("ttsContext error: %v", err)
	}
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Fatalf("expected no deadline for zero timeout")
	}
}
