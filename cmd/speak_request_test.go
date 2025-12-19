package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newSpeakTestCommand(t *testing.T) (*cobra.Command, *speakOptions) {
	t.Helper()
	opts := &speakOptions{
		modelID:   "eleven_multilingual_v2",
		outputFmt: "mp3_44100_128",
		speed:     1.0,
	}
	cmd := &cobra.Command{Use: "speak"}
	cmd.Flags().Float64Var(&opts.stability, "stability", 0, "")
	cmd.Flags().Float64Var(&opts.similarity, "similarity", 0, "")
	cmd.Flags().Float64Var(&opts.similarity, "similarity-boost", 0, "")
	cmd.Flags().Float64Var(&opts.style, "style", 0, "")
	cmd.Flags().BoolVar(&opts.speakerBoost, "speaker-boost", false, "")
	cmd.Flags().BoolVar(&opts.noSpeakerBoost, "no-speaker-boost", false, "")
	cmd.Flags().Uint64Var(&opts.seed, "seed", 0, "")
	cmd.Flags().StringVar(&opts.normalize, "normalize", "", "")
	cmd.Flags().StringVar(&opts.lang, "lang", "", "")
	return cmd, opts
}

func TestBuildTTSRequest_DefaultsOmitOptionalFields(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)

	req, err := buildTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildTTSRequest error: %v", err)
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

func TestBuildTTSRequest_SimilarityBoostAlias(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--similarity-boost", "0.9"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildTTSRequest error: %v", err)
	}
	if req.VoiceSettings.SimilarityBoost == nil || *req.VoiceSettings.SimilarityBoost != 0.9 {
		t.Fatalf("expected similarity_boost 0.9, got %#v", req.VoiceSettings.SimilarityBoost)
	}
}

func TestBuildTTSRequest_SpeakerBoostSetsJSONKey(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--speaker-boost"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildTTSRequest error: %v", err)
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

func TestBuildTTSRequest_InvalidNormalize(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--normalize", "wat"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "normalize must be one of") {
		t.Fatalf("expected normalize error, got %v", err)
	}
}

func TestBuildTTSRequest_InvalidLang(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--lang", "eng"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "lang must be a 2-letter") {
		t.Fatalf("expected lang error, got %v", err)
	}
}

func TestBuildTTSRequest_InvalidSeed(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--seed", "4294967296"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "seed must be between") {
		t.Fatalf("expected seed error, got %v", err)
	}
}

func TestBuildTTSRequest_SpeakerBoostConflict(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--speaker-boost", "--no-speaker-boost"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "choose only one") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestBuildTTSRequest_V3StabilityPresetsOnly(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	opts.modelID = "eleven_v3"
	if err := cmd.Flags().Parse([]string{"--stability", "0.55"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, err := buildTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "for eleven_v3, stability must be one of") {
		t.Fatalf("expected v3 stability preset error, got %v", err)
	}
}
