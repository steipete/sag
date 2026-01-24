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
		modelID:   "speech-01",
		outputFmt: "mp3_44100_128",
		speed:     1.0,
		voiceID:   "voice-1",
		stream:    false,
	}
	cmd := &cobra.Command{Use: "speak"}
	cmd.Flags().StringVar(&opts.normalize, "normalize", "", "")
	cmd.Flags().StringVar(&opts.lang, "lang", "", "")
	cmd.Flags().StringVar(&opts.emotion, "emotion", "", "")
	cmd.Flags().IntVar(&opts.pitch, "pitch", 0, "")
	cmd.Flags().Float64Var(&opts.volume, "volume", 0, "")
	cmd.Flags().StringVar(&opts.outputFmt, "format", opts.outputFmt, "")
	return cmd, opts
}

func TestBuildTTSRequest_DefaultsOmitOptionalFields(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)

	req, err := buildTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildTTSRequest error: %v", err)
	}

	if req.OutputFormat != "mp3" {
		t.Fatalf("expected output format mp3, got %q", req.OutputFormat)
	}
	if req.AudioSetting == nil || req.AudioSetting.SampleRate == nil || req.AudioSetting.Bitrate == nil {
		t.Fatalf("expected audio settings populated")
	}
	if *req.AudioSetting.SampleRate != 44100 || *req.AudioSetting.Bitrate != 128000 {
		t.Fatalf("unexpected audio settings: %+v", req.AudioSetting)
	}
	if req.LanguageBoost != "" {
		t.Fatalf("expected language_boost to be empty, got %q", req.LanguageBoost)
	}
	if req.VoiceSetting == nil || req.VoiceSetting.Speed == nil {
		t.Fatalf("expected voice_setting.speed to be set")
	}
	if req.VoiceSetting.TextNormalization != nil || req.VoiceSetting.Vol != nil || req.VoiceSetting.Pitch != nil || req.VoiceSetting.Emotion != "" {
		t.Fatalf("expected optional voice settings to be omitted")
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "text_normalization") || strings.Contains(s, "vol") || strings.Contains(s, "pitch") || strings.Contains(s, "emotion") {
		t.Fatalf("expected optional fields to be omitted, got %s", s)
	}
}

func TestBuildTTSRequest_NormalizeOnSetsBool(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--normalize", "on"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildTTSRequest error: %v", err)
	}
	if req.VoiceSetting.TextNormalization == nil || *req.VoiceSetting.TextNormalization != true {
		t.Fatalf("expected text_normalization true, got %#v", req.VoiceSetting.TextNormalization)
	}
}

func TestBuildTTSRequest_NormalizeAutoOmits(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{"--normalize", "auto"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildTTSRequest error: %v", err)
	}
	if req.VoiceSetting.TextNormalization != nil {
		t.Fatalf("expected text_normalization omitted, got %#v", req.VoiceSetting.TextNormalization)
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

func TestBuildTTSRequest_LangEmptyError(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Set("lang", " "); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	_, err := buildTTSRequest(cmd, *opts, "hello")
	if err == nil || !strings.Contains(err.Error(), "lang must be non-empty") {
		t.Fatalf("expected lang error, got %v", err)
	}
}

func TestBuildTTSRequest_VoiceSettingFields(t *testing.T) {
	cmd, opts := newSpeakTestCommand(t)
	if err := cmd.Flags().Parse([]string{
		"--emotion", "happy",
		"--pitch", "2",
		"--volume", "1.5",
		"--lang", "en",
		"--format", "mp3_48000_96",
	}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	req, err := buildTTSRequest(cmd, *opts, "hello")
	if err != nil {
		t.Fatalf("buildTTSRequest error: %v", err)
	}
	if req.LanguageBoost != "en" {
		t.Fatalf("expected language_boost en, got %q", req.LanguageBoost)
	}
	if req.VoiceSetting.Emotion != "happy" {
		t.Fatalf("expected emotion happy, got %q", req.VoiceSetting.Emotion)
	}
	if req.VoiceSetting.Pitch == nil || *req.VoiceSetting.Pitch != 2 {
		t.Fatalf("expected pitch 2, got %#v", req.VoiceSetting.Pitch)
	}
	if req.VoiceSetting.Vol == nil || *req.VoiceSetting.Vol != 1.5 {
		t.Fatalf("expected volume 1.5, got %#v", req.VoiceSetting.Vol)
	}
	if req.AudioSetting == nil || req.AudioSetting.SampleRate == nil || req.AudioSetting.Bitrate == nil {
		t.Fatalf("expected audio settings")
	}
	if *req.AudioSetting.SampleRate != 48000 || *req.AudioSetting.Bitrate != 96000 {
		t.Fatalf("unexpected audio settings: %+v", req.AudioSetting)
	}
}
