package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/steipete/sag/internal/elevenlabs"
)

func TestInferFormatFromExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"out.mp3", "mp3_44100_128"},
		{"out.MP3", "mp3_44100_128"},
		{"audio.wav", "pcm_44100"},
		{"audio.WAVE", "pcm_44100"},
		{"audio.unknown", ""},
	}
	for _, tt := range tests {
		if got := inferFormatFromExt(tt.path); got != tt.want {
			t.Fatalf("inferFormatFromExt(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestResolveTextFromArgs(t *testing.T) {
	got, err := resolveText([]string{"hello", "world"}, "")
	if err != nil {
		t.Fatalf("resolveText args error: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("resolveText args = %q, want %q", got, "hello world")
	}
}

func TestResolveTextFromFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "sag_text")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString("from file"); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}

	got, err := resolveText(nil, tmp.Name())
	if err != nil {
		t.Fatalf("resolveText file error: %v", err)
	}
	if got != "from file" {
		t.Fatalf("resolveText file = %q, want %q", got, "from file")
	}
}

func TestResolveTextFromStdin(t *testing.T) {
	orig := os.Stdin
	defer func() { os.Stdin = orig }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString("from stdin"); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	os.Stdin = r

	got, err := resolveText(nil, "")
	if err != nil {
		t.Fatalf("resolveText stdin error: %v", err)
	}
	if got != "from stdin" {
		t.Fatalf("resolveText stdin = %q, want %q", got, "from stdin")
	}
}

func TestResolveVoiceDefaultsToFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{"voices":[{"voice_id":"id1","name":"Alpha","category":"premade"},{"voice_id":"id2","name":"Beta","category":"premade"}]}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := elevenlabs.NewClient("key", srv.URL)
	id, err := resolveVoice(context.Background(), client, "")
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != "id1" {
		t.Fatalf("resolveVoice default id = %q, want id1", id)
	}
}

func TestResolveVoiceByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ensure search param contains name
		if !strings.Contains(r.URL.RawQuery, "search=roger") {
			t.Fatalf("expected search param to contain 'roger', got %s", r.URL.RawQuery)
		}
		if _, err := w.Write([]byte(`{"voices":[{"voice_id":"id-roger","name":"Roger","category":"premade"}]}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := elevenlabs.NewClient("key", srv.URL)
	id, err := resolveVoice(context.Background(), client, "roger")
	if err != nil {
		t.Fatalf("resolveVoice error: %v", err)
	}
	if id != "id-roger" {
		t.Fatalf("resolveVoice by name = %q, want id-roger", id)
	}
}
