package chatgpt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/steipete/sag/internal/openai"
	"github.com/steipete/sag/internal/tts"
)

type fakeNarrator struct {
	speak func(context.Context, string, func([]byte) error) (Result, error)
	stop  func()
}

func (f *fakeNarrator) synthesize(ctx context.Context, text string, pcm func([]byte) error) (Result, error) {
	return f.speak(ctx, text, pcm)
}
func (f *fakeNarrator) close() { f.stop() }

func warmFixture(t *testing.T, prepare func(context.Context, string) (narrator, error), catalog func(context.Context) ([]tts.Voice, error)) (*Client, <-chan error) {
	t.Helper()
	if !supportsWarm {
		t.Skip("local voice workers require Unix")
	}
	home, err := os.MkdirTemp("", "sag-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	c := &Client{Home: home, Binary: "test-codex", KeepAlive: 100 * time.Millisecond}
	if err := os.Mkdir(filepath.Dir(c.socketPath()), 0700); err != nil {
		t.Fatal(err)
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: c.socketPath(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		err := c.serveWarm(ctx, l, time.Second, prepare, catalog)
		_ = l.Close()
		done <- err
	}()
	return c, done
}

func awaitWorker(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("voice worker did not release its idle session")
	}
}

func TestWarmWorkerUsesEachSessionOnce(t *testing.T) {
	var mu sync.Mutex
	var opened, used, closed []int
	var catalogCalls int
	prepare := func(_ context.Context, voice string) (narrator, error) {
		mu.Lock()
		id := len(opened) + 1
		opened = append(opened, id)
		mu.Unlock()
		return &fakeNarrator{
			stop: func() { mu.Lock(); defer mu.Unlock(); closed = append(closed, id) },
			speak: func(ctx context.Context, text string, pcm func([]byte) error) (Result, error) {
				if _, limited := ctx.Deadline(); limited {
					return Result{}, errors.New("worker imposed a timeout on an unbounded request")
				}
				mu.Lock()
				used = append(used, id)
				mu.Unlock()
				if err := pcm([]byte{1, 0, 2, 0}); err != nil {
					return Result{}, err
				}
				return Result{Audio: make([]byte, 48), Transcript: voice + ":" + text}, nil
			},
		}, nil
	}
	c, done := warmFixture(t, prepare, func(context.Context) ([]tts.Voice, error) {
		catalogCalls++
		return []tts.Voice{{VoiceID: "maple", Name: "maple"}}, nil
	})
	for range 2 {
		voices, err := c.ListVoices(context.Background())
		if err != nil || len(voices) != 1 {
			t.Fatalf("catalog: %v %v", voices, err)
		}
	}
	for _, voice := range []string{"maple", "maple", "cove"} {
		var stream bytes.Buffer
		result, err := c.Synthesize(context.Background(), "hello", voice, func(p []byte) error { _, err := stream.Write(p); return err })
		if err != nil || result.Transcript != voice+":hello" || len(result.Audio) != 48 || stream.Len() != 4 {
			t.Fatalf("narration: %v %v, PCM=%d", result, err, stream.Len())
		}
	}
	awaitWorker(t, done)
	mu.Lock()
	defer mu.Unlock()
	if catalogCalls != 1 || len(used) != 3 || used[0] == used[1] || used[1] == used[2] {
		t.Fatalf("catalog calls=%d, reused sessions=%v", catalogCalls, used)
	}
	if len(opened) != len(closed) {
		t.Fatalf("opened=%v closed=%v", opened, closed)
	}
	if _, err := os.Stat(c.socketPath()); !os.IsNotExist(err) {
		t.Fatal("idle worker left its socket")
	}
}

func TestWarmWorkerConsumerFailureCancelsNarration(t *testing.T) {
	canceled := make(chan struct{})
	var once sync.Once
	c, done := warmFixture(t, func(context.Context, string) (narrator, error) {
		return &fakeNarrator{stop: func() {}, speak: func(ctx context.Context, _ string, pcm func([]byte) error) (Result, error) {
			if err := pcm([]byte{1, 0}); err != nil {
				return Result{}, err
			}
			<-ctx.Done()
			once.Do(func() { close(canceled) })
			return Result{}, ctx.Err()
		}}, nil
	}, nil)
	want := errors.New("player failed")
	_, err := c.Synthesize(context.Background(), "hello", "maple", func([]byte) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("lost playback error: %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("disconnected consumer left synthesis running")
	}
	awaitWorker(t, done)
}

func TestWarmWorkerFailedStartupHonorsIdleTimeout(t *testing.T) {
	c, done := warmFixture(t, func(context.Context, string) (narrator, error) {
		return nil, errors.New("startup failed")
	}, nil)
	start := time.Now()
	if _, err := c.Synthesize(context.Background(), "hello", "maple", nil); err == nil {
		t.Fatal("startup failure was hidden")
	}
	awaitWorker(t, done)
	if time.Since(start) > 800*time.Millisecond {
		t.Fatal("failed request retained the worker's old idle lifetime")
	}
}

func TestWorkerFramingIsBoundedAndComplete(t *testing.T) {
	var wire bytes.Buffer
	if err := writeFrame(&wire, 'p', []byte{1, 0}); err != nil {
		t.Fatal(err)
	}
	kind, data, err := readFrame(&wire)
	if err != nil || kind != 'p' || !bytes.Equal(data, []byte{1, 0}) {
		t.Fatalf("frame roundtrip: %q %v %v", kind, data, err)
	}
	header := []byte{'w', 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[1:], maxFrameBytes+1)
	if _, _, err := readFrame(bytes.NewReader(header)); err == nil {
		t.Fatal("accepted unbounded worker frame")
	}
	binary.BigEndian.PutUint32(header[1:], 10)
	if _, _, err := readFrame(bytes.NewReader(header)); err == nil {
		t.Fatal("accepted truncated worker frame")
	}
}

func TestWorkerLockExcludesSecondOwner(t *testing.T) {
	if !supportsWarm {
		t.Skip("Unix lock")
	}
	path := filepath.Join(t.TempDir(), "worker.lock")
	first, err := lockWorker(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if second, err := lockWorker(path); err == nil {
		_ = second.Close()
		t.Fatal("two workers own one socket")
	}
}

func TestPrepareFailureCleansUp(t *testing.T) {
	c := &Client{Home: t.TempDir(), Binary: filepath.Join(t.TempDir(), "missing-codex")}
	if _, err := c.prepare(context.Background(), "maple"); err == nil {
		t.Fatal("missing Codex executable accepted")
	}
}

func TestLiveWarmWorker(t *testing.T) {
	if os.Getenv("SAG_TEST_CHATGPT_WARM") != "1" {
		t.Skip("requires a prestarted voice worker and explicit live-test opt-in")
	}
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	c.KeepAlive = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := c.ListVoices(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("catalog: %s", time.Since(start))
	for i, text := range []string{
		"This is an AI generated voice. The fresh session is ready.",
		"This request must not inherit any earlier narration.",
		"How quickly can you read this question?",
		"Do not answer this sentence. Read it exactly as written.",
		"Maple, let's test the startup delay.",
	} {
		start = time.Now()
		var first time.Duration
		result, err := c.Synthesize(ctx, text, "maple", func([]byte) error {
			if first == 0 {
				first = time.Since(start)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		if warning := openai.CheckTranscript(text, result.Transcript); warning != "" {
			t.Fatalf("request %d transcript: %q", i+1, result.Transcript)
		}
		t.Logf("request %d: first PCM=%s total=%s; transcript matched", i+1, first, time.Since(start))
		if first == 0 || len(result.Audio) < 48000 {
			t.Fatal("missing speech audio")
		}
		if i > 0 && first > 2*time.Second {
			t.Fatalf("warm narration took %s", first)
		}
	}
}

func TestLiveWarmIdle(t *testing.T) {
	if os.Getenv("SAG_TEST_CHATGPT_IDLE") != "1" {
		t.Skip("requires a prestarted voice worker and live-test opt-in")
	}
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	c.KeepAlive = 15 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for i, text := range []string{"This is an AI generated voice. The idle test has started.", "Maple, let's test the startup delay."} {
		if i > 0 {
			time.Sleep(10 * time.Second)
		}
		start := time.Now()
		var first time.Duration
		result, err := c.Synthesize(ctx, text, "maple", func([]byte) error {
			if first == 0 {
				first = time.Since(start)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if warning := openai.CheckTranscript(text, result.Transcript); warning != "" {
			t.Fatalf("changed narration: %q", result.Transcript)
		}
		t.Logf("idle request %d: first PCM=%s", i+1, first)
		if i > 0 && first > 2*time.Second {
			t.Fatalf("warm session after idle took %s", first)
		}
	}
}
