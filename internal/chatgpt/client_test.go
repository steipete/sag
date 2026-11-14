package chatgpt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsolatedEnv(t *testing.T) {
	for _, key := range []string{"CODEX_HOME", "CODEX_API_KEY", "OPENAI_API_KEY", "OPENAI_API_KEY_FILE", "CODEX_ACCESS_TOKEN"} {
		t.Setenv(key, "must-not-leak")
	}
	env := isolatedEnv("voice-only")
	for _, entry := range env {
		if strings.Contains(entry, "must-not-leak") {
			t.Fatal("inherited an existing credential setting")
		}
	}
	if !strings.Contains(strings.Join(env, "\n"), "CODEX_HOME=voice-only") {
		t.Fatal("missing isolated home")
	}
}

func TestProtectedHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "custom"))
	for _, path := range []string{filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "voice"), filepath.Join(home, "custom")} {
		if err := (&Client{Home: path}).validateHome(); err == nil {
			t.Fatalf("accepted protected home %s", path)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(filepath.Join(home, ".codex"), alias); err != nil {
		t.Skip(err)
	}
	if err := (&Client{Home: filepath.Join(alias, "voice")}).validateHome(); err == nil {
		t.Fatal("accepted symlink into protected home")
	}
}

func TestInputRejectedBeforeLogin(t *testing.T) {
	c := &Client{Home: filepath.Join(t.TempDir(), "missing"), Binary: "missing-codex"}
	for _, text := range []string{"", strings.Repeat("x", MaxTextBytes+1)} {
		if _, err := c.Synthesize(context.Background(), text, "maple", nil); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestRecordingWAV(t *testing.T) {
	var r recording
	samples := make([]int16, 5000)
	for i := 1000; i < 3000; i++ {
		samples[i] = 200
	}
	if err := r.append(samples); err != nil {
		t.Fatal(err)
	}
	wav, err := r.wav()
	if err != nil {
		t.Fatal(err)
	}
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatal("not a WAV")
	}
	if binary.LittleEndian.Uint32(wav[24:28]) != 24000 {
		t.Fatal("wrong sample rate")
	}
	if int(binary.LittleEndian.Uint32(wav[40:44])) != len(wav)-44 {
		t.Fatal("incorrect WAV length")
	}
}

func TestRecordingStreamsBeforeCompletion(t *testing.T) {
	var r recording
	var stream bytes.Buffer
	if err := r.append(make([]int16, sampleRate)); err != nil {
		t.Fatal(err)
	}
	if err := r.writePCM(func(p []byte) error { _, err := stream.Write(p); return err }); err != nil {
		t.Fatal(err)
	}
	if stream.Len() != 0 {
		t.Fatal("streamed startup silence")
	}
	first := []int16{200, -300, 400}
	if err := r.append(first); err != nil {
		t.Fatal(err)
	}
	if err := r.writePCM(func(p []byte) error {
		// The decoder remains able to append while the consumer is running.
		if err := r.append([]int16{500}); err != nil {
			return err
		}
		_, err := stream.Write(p)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if r.drained() {
		t.Fatal("streaming waited until completion")
	}
	if stream.Len() != (sampleRate/10+len(first))*2 {
		t.Fatalf("wrong preroll: %d", stream.Len())
	}
	if err := r.writePCM(func(p []byte) error { _, err := stream.Write(p); return err }); err != nil {
		t.Fatal(err)
	}
	want := []byte{200, 0, 212, 254, 144, 1, 244, 1}
	if !bytes.Equal(stream.Bytes()[sampleRate/10*2:], want) {
		t.Fatal("lost, duplicated, or reordered PCM")
	}
	if err := r.writePCM(func([]byte) error { t.Fatal("replayed consumed samples"); return nil }); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("speaker unavailable")
	if err := r.append([]int16{600}); err != nil {
		t.Fatal(err)
	}
	if err := r.writePCM(func([]byte) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("lost consumer error: %v", err)
	}
}

func TestLiveMapleAccount(t *testing.T) {
	if os.Getenv("SAG_TEST_CHATGPT_LIVE") != "1" {
		t.Skip("requires explicit live-test opt-in and a separate ChatGPT sign-in")
	}
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	const text = "This is an AI generated voice. The Maple voice test is working."
	start := time.Now()
	if _, err := c.ListVoices(ctx); err != nil {
		t.Fatal(err)
	}
	catalogTime := time.Since(start)
	var firstAudio time.Duration
	var pcmBytes int
	result, err := c.Synthesize(ctx, text, "maple", func(pcm []byte) error {
		if firstAudio == 0 {
			firstAudio = time.Since(start)
		}
		pcmBytes += len(pcm)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Transcript) != text {
		t.Fatalf("transcript mismatch: %q", result.Transcript)
	}
	if len(result.Audio) < 48000 {
		t.Fatalf("insufficient audio: %d bytes", len(result.Audio))
	}
	if firstAudio == 0 || pcmBytes < len(result.Audio)-44 {
		t.Fatalf("incomplete stream: first=%s pcm=%d wav=%d", firstAudio, pcmBytes, len(result.Audio))
	}
	t.Logf("Maple: catalog=%s; first PCM=%s; complete=%s; %d WAV bytes; transcript matched", catalogTime, firstAudio, time.Since(start), len(result.Audio))
}
