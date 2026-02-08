package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

const playbackPollInterval = 100 * time.Millisecond

var (
	audioCtxMu      sync.Mutex
	audioCtx        *oto.Context
	audioReady      chan struct{}
	audioSampleRate int
	audioContextErr error
)

// StreamViaOto decodes MP3 audio from the reader and plays it via the oto backend (CoreAudio/ALSA/etc.).
func StreamViaOto(ctx context.Context, r io.Reader) error {
	decoder, err := mp3.NewDecoder(r)
	if err != nil {
		return fmt.Errorf("decode mp3: %w", err)
	}

	const (
		channelCount = 2
		format       = oto.FormatSignedInt16LE
	)

	audioCtx, ready, err := getAudioContext(decoder.SampleRate(), channelCount, format)
	if err != nil {
		return fmt.Errorf("audio context: %w", err)
	}
	if ready != nil {
		<-ready
	}

	player := audioCtx.NewPlayer(decoder)
	defer func() {
		_ = player.Close()
	}()
	player.Play()

	return waitForPlayback(ctx, player)
}

func getAudioContext(sampleRate, channelCount int, format oto.Format) (*oto.Context, chan struct{}, error) {
	audioCtxMu.Lock()
	defer audioCtxMu.Unlock()

	if audioCtx != nil {
		if audioContextErr != nil {
			return nil, nil, audioContextErr
		}
		if audioSampleRate != sampleRate {
			return nil, nil, fmt.Errorf("context already initialized at %d Hz; got %d Hz", audioSampleRate, sampleRate)
		}
		return audioCtx, audioReady, nil
	}

	if sampleRate <= 0 {
		return nil, nil, errors.New("invalid sample rate")
	}

	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: channelCount,
		Format:       format,
	})
	if err != nil {
		audioContextErr = err
		return nil, nil, err
	}
	audioCtx = ctx
	audioReady = ready
	audioSampleRate = sampleRate
	audioContextErr = nil
	return audioCtx, audioReady, nil
}

func waitForPlayback(ctx context.Context, player *oto.Player) error {
	ticker := time.NewTicker(playbackPollInterval)
	defer ticker.Stop()

	for {
		if !player.IsPlaying() {
			return nil
		}
		select {
		case <-ctx.Done():
			player.Pause()
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
