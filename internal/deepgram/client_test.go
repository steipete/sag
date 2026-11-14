package deepgram

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Token test-key" {
			t.Errorf("wrong catalog request")
		}
		_, _ = io.WriteString(w, `{"tts":[{"canonical_name":"aura-2-thalia-en","name":"thalia","architecture":"aura-2","languages":["en"],"metadata":{"display_name":"Thalia","accent":"American","sample":"https://example.com/sample.wav","tags":["clear"]}},{"canonical_name":"flux-1","name":"flux"},{"canonical_name":"aura-2-apollo-en","name":"Apollo"}]}`)
	}))
	defer srv.Close()
	c := NewClient("test-key", srv.URL+"/v1")
	voices, err := c.ListVoices(context.Background())
	if err != nil || len(voices) != 2 || voices[0].VoiceID != "aura-2-apollo-en" {
		t.Fatalf("catalog: %+v %v", voices, err)
	}
	voice, err := c.GetVoice(context.Background(), DefaultVoice)
	if err != nil || voice.Name != "Thalia" || voice.PreviewURL != "https://example.com/sample.wav" || voice.Labels["language"] != "en" {
		t.Fatalf("metadata: %+v %v", voice, err)
	}
	voices, err = c.SearchVoices(context.Background(), "THALIA", 1)
	if err != nil || len(voices) != 1 || voices[0].VoiceID != DefaultVoice {
		t.Fatalf("search: %+v %v", voices, err)
	}
}

func TestSynthesizeStreamsBeforeCompletion(t *testing.T) {
	first := make(chan struct{})
	finish := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/speak" || r.Header.Get("Authorization") != "Token test-key" {
			t.Errorf("wrong speech request")
		}
		q := r.URL.Query()
		if q.Get("model") != DefaultVoice || q.Get("encoding") != "linear16" || q.Get("sample_rate") != "24000" || q.Get("container") != "none" || q.Get("speed") != "1.2" {
			t.Errorf("wrong query: %s", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"text":"hello"}` {
			t.Errorf("wrong body: %s", body)
		}
		w.Header().Set("Content-Type", "audio/l16;rate=24000")
		_, _ = w.Write([]byte{1, 2, 3})
		w.(http.Flusher).Flush()
		select {
		case <-finish:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte{4, 5, 6})
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var streamed []byte
	wav, err := NewClient("test-key", srv.URL+"/v1").Synthesize(ctx, "hello", "", 1.2, func(pcm []byte) error {
		if len(pcm)%2 != 0 {
			t.Errorf("partial sample delivered")
		}
		streamed = append(streamed, pcm...)
		select {
		case <-first:
		default:
			close(first)
			close(finish)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(streamed, []byte{1, 2, 3, 4, 5, 6}) || !bytes.Equal(wav[44:], streamed) || binary.LittleEndian.Uint32(wav[24:28]) != 24000 {
		t.Fatalf("invalid PCM/WAV: %v", streamed)
	}
}

func TestSynthesizeFailureDoesNotReturnWAV(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            int
		contentType, body string
		callbackError     bool
	}{
		{"unauthorized", 401, "application/json", "private error body", false},
		{"wrong format", 200, "audio/wav", "RIFF", false},
		{"empty", 200, "audio/l16;rate=24000", "", false},
		{"odd sample", 200, "audio/l16;rate=24000", "abc", false},
		{"playback failed", 200, "audio/l16;rate=24000", "abcd", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			wav, err := NewClient("secret-key", srv.URL).Synthesize(context.Background(), "hello", "", 1, func([]byte) error {
				if tc.callbackError {
					return errors.New("player failed")
				}
				return nil
			})
			if err == nil || wav != nil || strings.Contains(err.Error(), "secret-key") || strings.Contains(err.Error(), "private error body") {
				t.Fatalf("invalid failure: audio=%d err=%v", len(wav), err)
			}
		})
	}
}

func TestSynthesizeCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/l16;rate=24000")
		_, _ = w.Write([]byte{1, 2})
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	wav, err := NewClient("test-key", srv.URL).Synthesize(ctx, "hello", "", 1, func([]byte) error { cancel(); return ctx.Err() })
	if !errors.Is(err, context.Canceled) || wav != nil {
		t.Fatalf("cancel: audio=%d err=%v", len(wav), err)
	}
}

func TestLiveSynthesis(t *testing.T) {
	if os.Getenv("SAG_TEST_DEEPGRAM_LIVE") != "1" {
		t.Skip("requires explicit live-test opt-in and a test-account API key")
	}
	c := NewClient(os.Getenv("DEEPGRAM_API_KEY"), "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	voices, err := c.SearchVoices(ctx, "thalia", 1)
	if err != nil || len(voices) != 1 {
		t.Fatalf("live catalog: %v", err)
	}
	start := time.Now()
	var first time.Duration
	var streamed int
	wav, err := c.Synthesize(ctx, "This is an AI generated voice. Testing a quick and clear response.", DefaultVoice, 1, func(pcm []byte) error {
		if streamed == 0 {
			first = time.Since(start)
		}
		streamed += len(pcm)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if streamed != len(wav)-44 || streamed == 0 {
		t.Fatal("incomplete live audio")
	}
	t.Logf("voice=%s first_pcm=%s complete=%s bytes=%d", DefaultVoice, first, time.Since(start), streamed)
}
