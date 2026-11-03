package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/steipete/sag/internal/openai"
)

func TestOpenAIPlaysBeforeCompletionAndSavesValidatedWAV(t *testing.T) {
	resetProviderEnv(t)
	resetRootCommandState()
	t.Cleanup(resetRootCommandState)
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("SAG_PLAYER", "")
	consumed := make(chan struct{})
	out := filepath.Join(t.TempDir(), "voice.wav")
	const oldOutput = "previous valid file"
	if err := os.WriteFile(out, []byte(oldOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, _, err := conn.Read(ctx); err != nil {
			t.Error(err)
			return
		}
		for _, delta := range []string{"AQ==", "AgME"} {
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.output_audio.delta","delta":"`+delta+`"}`)); err != nil {
				t.Error(err)
				return
			}
		}
		// Completion is withheld until playback consumes real PCM frames.
		select {
		case <-consumed:
		case <-ctx.Done():
			t.Error("playback waited for complete response")
			return
		}
		for _, event := range []string{
			`{"type":"response.output_audio_transcript.done","transcript":"Hello world"}`,
			`{"type":"response.output_audio.done"}`,
			`{"type":"response.done","response":{"status":"completed"}}`,
		} {
			if err := conn.Write(ctx, websocket.MessageText, []byte(event)); err != nil {
				t.Error(err)
				return
			}
		}
	}))
	defer srv.Close()
	original := playOpenAIPCM
	t.Cleanup(func() { playOpenAIPCM = original })
	playOpenAIPCM = func(_ context.Context, r io.ReadCloser) error {
		first := make([]byte, 4)
		if _, err := io.ReadFull(r, first); err != nil {
			return err
		}
		if !bytes.Equal(first, []byte{1, 2, 3, 4}) {
			t.Errorf("PCM=%v", first)
		}
		data, err := os.ReadFile(out)
		if err != nil || string(data) != oldOutput {
			t.Errorf("output changed before validation: %v", err)
		}
		close(consumed)
		_, err = io.Copy(io.Discard, r)
		return err
	}
	rootCmd.SetArgs([]string{"speak", "--provider", "openai", "--base-url", srv.URL, "--play", "-o", out, "--timeout", "5s", "Hello world"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil || len(data) != 48 || !bytes.Equal(data[44:], []byte{1, 2, 3, 4}) {
		t.Fatalf("invalid saved WAV: len=%d err=%v", len(data), err)
	}
}

func TestOpenAIStreamPlayerFailureStopsSynthesis(t *testing.T) {
	for _, blockedWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "network read", true: "audio write"}[blockedWrite], func(t *testing.T) {
			original := playOpenAIPCM
			t.Cleanup(func() { playOpenAIPCM = original })
			started := make(chan struct{})
			failure := errors.New("device unavailable")
			playOpenAIPCM = func(context.Context, io.ReadCloser) error { <-started; return failure }
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			out := filepath.Join(t.TempDir(), "voice.wav")
			const previous = "keep this file"
			if err := os.WriteFile(out, []byte(previous), 0o600); err != nil {
				t.Fatal(err)
			}
			joined := false
			_, err := streamOpenAIAndPlay(ctx, speakOptions{play: true, outputPath: out}, func(ctx context.Context, onPCM func([]byte) error) (openai.Result, error) {
				defer func() { joined = true }()
				close(started)
				if blockedWrite {
					return openai.Result{}, onPCM([]byte{1, 2})
				}
				<-ctx.Done()
				return openai.Result{}, ctx.Err()
			})
			if !errors.Is(err, failure) || !joined {
				t.Fatalf("err=%v joined=%t", err, joined)
			}
			data, err := os.ReadFile(out)
			if err != nil || string(data) != previous {
				t.Fatal("failed playback replaced output")
			}
		})
	}
}

func TestOpenAIStreamPreservesProviderFailure(t *testing.T) {
	original := playOpenAIPCM
	t.Cleanup(func() { playOpenAIPCM = original })
	playOpenAIPCM = func(_ context.Context, r io.ReadCloser) error { _, err := io.Copy(io.Discard, r); return err }
	failure := errors.New("incomplete response")
	_, err := streamOpenAIAndPlay(context.Background(), speakOptions{play: true}, func(_ context.Context, onPCM func([]byte) error) (openai.Result, error) {
		if err := onPCM([]byte{1, 2}); err != nil {
			return openai.Result{}, err
		}
		return openai.Result{}, failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("lost provider error: %v", err)
	}
}

func TestOpenAIStreamSelection(t *testing.T) {
	for _, tc := range []struct {
		player, env string
		stream      bool
	}{
		{"auto", "", true}, {"oto", "", true}, {"afplay", "", false}, {"auto", "afplay", false}, {"auto", "oto", true},
	} {
		t.Run(tc.player+"/"+tc.env, func(t *testing.T) {
			resetRootCommandState()
			t.Cleanup(resetRootCommandState)
			t.Setenv("SAG_PLAYER", tc.env)
			cmd, _, _ := rootCmd.Find([]string{"speak"})
			opts := speakOptions{play: true, stream: true, player: tc.player}
			if err := prepareOpenAIOptions(cmd, &opts); err != nil {
				t.Fatal(err)
			}
			if opts.stream != tc.stream {
				t.Fatalf("stream=%t want %t", opts.stream, tc.stream)
			}
			if tc.stream && !strings.Contains(opts.outputFmt, "wav") {
				t.Fatal("lost WAV format")
			}
		})
	}
}
