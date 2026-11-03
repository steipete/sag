package cmd

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steipete/sag/internal/audio"
	"github.com/steipete/sag/internal/openai"
)

func prepareOpenAIOptions(cmd *cobra.Command, opts *speakOptions) error {
	for _, flag := range []string{"stability", "similarity", "similarity-boost", "style", "speaker-boost", "no-speaker-boost", "seed", "normalize", "lang", "latency-tier"} {
		if cmd.Flags().Changed(flag) {
			return fmt.Errorf("OpenAI Realtime does not support --%s", flag)
		}
	}
	if !cmd.Flags().Changed("model-id") {
		opts.modelID = openai.DefaultModel
	}
	if cmd.Flags().Changed("format") {
		if !strings.EqualFold(opts.outputFmt, "wav") {
			return fmt.Errorf("OpenAI Realtime supports wav output only; got %s", opts.outputFmt)
		}
	} else if ext := strings.ToLower(filepath.Ext(opts.outputPath)); ext != "" && ext != ".wav" {
		return fmt.Errorf("OpenAI Realtime supports .wav output only; got %s", ext)
	}
	opts.outputFmt = "wav"
	if opts.play {
		if _, err := playbackFunc(opts.player); err != nil {
			return err
		}
	} else {
		opts.stream = false
	}
	if opts.stream && resolvedPlayer(opts.player) == "afplay" {
		if cmd.Flags().Changed("stream") {
			return fmt.Errorf("OpenAI streaming requires --player oto; use --no-stream with afplay")
		}
		opts.stream = false
	}
	return nil
}

type openAISynthesizer func(context.Context, func([]byte) error) (openai.Result, error)

var playOpenAIPCM = func(ctx context.Context, r io.ReadCloser) error {
	return audio.StreamPCM16(ctx, r, 24000, 1)
}

func streamOpenAIAndPlay(ctx context.Context, opts speakOptions, synthesize openAISynthesizer) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = reader.CloseWithError(ctx.Err()); _ = writer.CloseWithError(ctx.Err()) })
	defer stop()
	type outcome struct {
		result openai.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := synthesize(ctx, func(pcm []byte) error { _, err := writer.Write(pcm); return err })
		_ = writer.CloseWithError(err)
		done <- outcome{result, err}
	}()
	playErr := playOpenAIPCM(ctx, reader)
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
		return int64(len(completed.result.Audio)), nil
	}
	opts.play = false
	return convertAndPlay(ctx, opts, func(context.Context) ([]byte, error) { return completed.result.Audio, nil })
}
