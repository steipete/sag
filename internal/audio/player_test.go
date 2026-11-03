package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestStreamViaOtoBadMP3(t *testing.T) {
	err := StreamViaOto(context.Background(), strings.NewReader("not-mp3"))
	if err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestAudioStartupCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForAudioReady(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) {
		t.Fatalf("startup ignored cancellation: %v", err)
	}
	ready := make(chan struct{})
	close(ready)
	if err := waitForAudioReady(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
}

type testPlayback struct {
	playing bool
	paused  bool
	err     error
}

func (p *testPlayback) IsPlaying() bool { return p.playing }
func (p *testPlayback) Pause()          { p.paused = true }
func (p *testPlayback) Err() error      { return p.err }

func TestPlaybackErrorAndCancellation(t *testing.T) {
	failure := errors.New("audio device failed")
	if err := waitForPlayback(context.Background(), &testPlayback{err: failure}); !errors.Is(err, failure) {
		t.Fatalf("lost player error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	player := &testPlayback{playing: true}
	if err := waitForPlayback(ctx, player); !errors.Is(err, context.Canceled) || !player.paused {
		t.Fatalf("cancellation=%v paused=%t", err, player.paused)
	}
}

func TestParsePCM16WAV(t *testing.T) {
	pcm := []byte{0, 0, 1, 0, 2, 0, 3, 0}
	wav := testWAV(t, 48000, 1, pcm)

	got, err := parsePCM16WAV(wav)
	if err != nil {
		t.Fatalf("parsePCM16WAV error: %v", err)
	}
	if got.sampleRate != 48000 || got.channels != 1 {
		t.Fatalf("unexpected format: sampleRate=%d channels=%d", got.sampleRate, got.channels)
	}
	if !bytes.Equal(got.data, pcm) {
		t.Fatalf("PCM = %v, want %v", got.data, pcm)
	}
}

func TestParsePCM16WAVRejectsUnsupportedOrTruncatedAudio(t *testing.T) {
	wav := testWAV(t, 48000, 1, []byte{0, 0})
	binary.LittleEndian.PutUint16(wav[20:22], 3)
	if _, err := parsePCM16WAV(wav); err == nil || !strings.Contains(err.Error(), "unsupported WAV format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}

	if _, err := parsePCM16WAV([]byte("RIFF")); err == nil || !strings.Contains(err.Error(), "missing RIFF/WAVE header") {
		t.Fatalf("expected truncated header error, got %v", err)
	}
}

func testWAV(t *testing.T, sampleRate, channels int, pcm []byte) []byte {
	t.Helper()
	wav := make([]byte, 44+len(pcm))
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(36+len(pcm)))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(wav[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(wav[28:32], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(wav[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav
}
