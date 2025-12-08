package elevenlabs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

func TestNewClientDefaultsBase(t *testing.T) {
	c := NewClient("key", "")
	if c.baseURL != "https://api.elevenlabs.io" {
		t.Fatalf("unexpected baseURL: %s", c.baseURL)
	}
}

func TestListVoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/voices" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if search := r.URL.Query().Get("search"); search != "roger" {
			t.Fatalf("expected search query 'roger', got %q", search)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"voices":[{"voice_id":"id1","name":"Roger","category":"premade"}]}`))
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	voices, err := c.ListVoices(context.Background(), "roger")
	if err != nil {
		t.Fatalf("ListVoices error: %v", err)
	}
	if len(voices) != 1 || voices[0].VoiceID != "id1" {
		t.Fatalf("unexpected voices: %+v", voices)
	}
}

func TestStreamTTS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/text-to-speech/voice123/stream") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "audio/mpeg" {
			t.Fatalf("missing Accept header")
		}
		_, _ = w.Write([]byte("audio-data"))
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	rc, err := c.StreamTTS(context.Background(), "voice123", TTSRequest{Text: "hi"}, 0)
	if err != nil {
		t.Fatalf("StreamTTS error: %v", err)
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	if string(b) != "audio-data" {
		t.Fatalf("unexpected body: %q", string(b))
	}
}

func TestStreamTTS_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	_, err := c.StreamTTS(context.Background(), "voice123", TTSRequest{Text: "hi"}, 0)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 error, got %v", err)
	}
}

func TestConvertTTS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path.Base(r.URL.Path) != "voice123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("full-audio"))
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	data, err := c.ConvertTTS(context.Background(), "voice123", TTSRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("ConvertTTS error: %v", err)
	}
	if string(data) != "full-audio" {
		t.Fatalf("unexpected data: %q", string(data))
	}
}

func TestConvertTTS_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	_, err := c.ConvertTTS(context.Background(), "voice123", TTSRequest{Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}
