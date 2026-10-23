package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type signaledStream struct {
	io.ReadCloser
	started chan struct{}
	once    sync.Once
}

func (r *signaledStream) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.ReadCloser.Read(p)
}

func TestStreamAndPlayStopsCopyWithPlayer(t *testing.T) {
	playerFailure := errors.New("player stopped")
	for _, blockedRead := range []bool{false, true} {
		name := "blocked write"
		if blockedRead {
			name = "blocked read"
		}
		t.Run(name, func(t *testing.T) {
			body := io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), 128*1024)))
			if blockedRead {
				reader, writer := io.Pipe()
				defer func() { _ = writer.Close() }()
				body = reader
			}
			source := &signaledStream{ReadCloser: body, started: make(chan struct{})}
			defer func() { _ = source.Close() }()
			var playbackReader io.Reader
			playerReady := make(chan struct{})
			original := playToSpeakers
			playToSpeakers = func(_ context.Context, r io.Reader) error {
				playbackReader = r
				close(playerReady)
				<-source.started
				if blockedRead {
					return playerFailure
				}
				return nil
			}
			defer func() { playToSpeakers = original }()
			done := make(chan error, 1)
			go func() {
				_, err := streamAndPlay(context.Background(), speakOptions{play: true}, func(context.Context) (io.ReadCloser, error) { return source, nil })
				done <- err
			}()
			<-playerReady
			select {
			case err := <-done:
				if blockedRead && !errors.Is(err, playerFailure) {
					t.Fatalf("got %v, want player error", err)
				}
				if !blockedRead && err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				_ = source.Close()
				_ = playbackReader.(io.Closer).Close()
				_ = <-done
				t.Fatal("copy did not stop after the player returned")
			}
		})
	}
}

type failedStream struct{ err error }

func (r failedStream) Read([]byte) (int, error) { return 0, r.err }
func (r failedStream) Close() error             { return nil }

func TestStreamAndPlayPreservesSourceError(t *testing.T) {
	want := errors.New("provider read failed")
	original := playToSpeakers
	playToSpeakers = func(_ context.Context, r io.Reader) error { _, err := io.Copy(io.Discard, r); return err }
	defer func() { playToSpeakers = original }()
	_, err := streamAndPlay(context.Background(), speakOptions{play: true}, func(context.Context) (io.ReadCloser, error) { return failedStream{want}, nil })
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want source error", err)
	}
}

func TestStreamAndPlayKeepsCompleteOutput(t *testing.T) {
	original := playToSpeakers
	playToSpeakers = func(_ context.Context, r io.Reader) error { _, err := io.Copy(io.Discard, r); return err }
	defer func() { playToSpeakers = original }()
	const data = "complete audio"
	n, err := streamAndPlay(context.Background(), speakOptions{play: true}, func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(data)), nil })
	if err != nil || n != int64(len(data)) {
		t.Fatalf("got bytes=%d err=%v", n, err)
	}
}

func TestStreamAndPlayRejectsInterruptedOutput(t *testing.T) {
	original := playToSpeakers
	playToSpeakers = func(context.Context, io.Reader) error { return nil }
	defer func() { playToSpeakers = original }()
	_, err := streamAndPlay(context.Background(), speakOptions{play: true, outputPath: t.TempDir() + "/audio.mp3"}, func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(io.LimitReader(strings.NewReader(strings.Repeat("a", 128*1024)), 128*1024)), nil
	})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("got %v, want interrupted-output error", err)
	}
}
