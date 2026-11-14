package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steipete/sag/internal/audio"
)

func preparePCMPlayback(cmd *cobra.Command, opts *speakOptions, provider string) error {
	if opts.play {
		if _, err := playbackFunc(opts.player); err != nil {
			return err
		}
	} else {
		opts.stream = false
	}
	if opts.stream && resolvedPlayer(opts.player) == "afplay" {
		if cmd.Flags().Changed("stream") {
			return fmt.Errorf("%s streaming requires --player oto; use --no-stream with afplay", provider)
		}
		opts.stream = false
	}
	return nil
}

type pcmSynthesizer func(context.Context, func([]byte) error) ([]byte, error)

var playPCM = func(ctx context.Context, r io.ReadCloser) error {
	return audio.StreamPCM16(ctx, r, 24000, 1)
}

func streamPCMAndPlay(ctx context.Context, opts speakOptions, synthesize pcmSynthesizer) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = reader.CloseWithError(ctx.Err()); _ = writer.CloseWithError(ctx.Err()) })
	defer stop()
	type outcome struct {
		result []byte
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		start := time.Now()
		first := true
		result, err := synthesize(ctx, func(pcm []byte) error {
			if first && len(pcm) > 0 {
				first = false
				if opts.metrics {
					fmt.Fprintf(os.Stderr, "metrics: first_pcm=%s\n", time.Since(start).Truncate(time.Millisecond))
				}
			}
			_, err := writer.Write(pcm)
			return err
		})
		_ = writer.CloseWithError(err)
		done <- outcome{result, err}
	}()
	playErr := playPCM(ctx, reader)
	_ = reader.Close()
	cancel()
	completed := <-done
	if playErr != nil {
		return 0, playErr
	}
	if completed.err != nil {
		return 0, completed.err
	}
	// Save only a complete response after successful playback.
	if opts.outputPath == "" {
		return int64(len(completed.result)), nil
	}
	opts.play = false
	return convertAndPlay(ctx, opts, func(context.Context) ([]byte, error) { return completed.result, nil })
}
