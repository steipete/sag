package audio

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"github.com/ebitengine/oto/v3"
)

// StreamToSpeakers decodes MP3 audio from the reader and plays it to the default output device.
func StreamToSpeakers(ctx context.Context, r io.Reader) error {
	decoder, err := mp3.NewDecoder(r)
	if err != nil {
		return fmt.Errorf("decode mp3: %w", err)
	}

	const (
		channelCount = 2
		format       = oto.FormatSignedInt16LE
	)

	audioCtx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:      decoder.SampleRate(),
		ChannelCount:    channelCount,
		Format:          format,
	})
	if err != nil {
		return fmt.Errorf("audio context: %w", err)
	}
	<-ready

	player := audioCtx.NewPlayer(decoder)
	player.Play()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = player.Close()
			return ctx.Err()
		case <-ticker.C:
			if !player.IsPlaying() {
				_ = player.Close()
				return nil
			}
		}
	}
}
