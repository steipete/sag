package elevenlabs

import (
	"context"
	"encoding/json"
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"voices":[{"voice_id":"id1","name":"Sarah","category":"premade"},{"voice_id":"id2","name":"Roger","category":"premade"}]}`))
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	voices, err := c.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices error: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got: %+v", voices)
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

func TestStreamTTS_PayloadFields(t *testing.T) {
	stability := 0.0
	similarity := 1.0
	style := 0.25
	speakerBoost := false
	speed := 1.1
	seed := uint32(0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if got["seed"] != float64(0) {
			t.Fatalf("expected seed 0, got %v", got["seed"])
		}
		if got["apply_text_normalization"] != "off" {
			t.Fatalf("expected apply_text_normalization off, got %v", got["apply_text_normalization"])
		}
		if got["language_code"] != "en" {
			t.Fatalf("expected language_code en, got %v", got["language_code"])
		}

		vs, ok := got["voice_settings"].(map[string]any)
		if !ok {
			t.Fatalf("expected voice_settings object, got %T", got["voice_settings"])
		}
		if vs["stability"] != float64(0) {
			t.Fatalf("expected stability 0, got %v", vs["stability"])
		}
		if vs["similarity_boost"] != float64(1) {
			t.Fatalf("expected similarity_boost 1, got %v", vs["similarity_boost"])
		}
		if vs["style"] != style {
			t.Fatalf("expected style %v, got %v", style, vs["style"])
		}
		if vs["use_speaker_boost"] != speakerBoost {
			t.Fatalf("expected use_speaker_boost %v, got %v", speakerBoost, vs["use_speaker_boost"])
		}
		if vs["speed"] != speed {
			t.Fatalf("expected speed %v, got %v", speed, vs["speed"])
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	_, err := c.StreamTTS(context.Background(), "voice123", TTSRequest{
		Text:                   "hi",
		ModelID:                "eleven_multilingual_v2",
		OutputFormat:           "mp3_44100_128",
		Seed:                   &seed,
		ApplyTextNormalization: "off",
		LanguageCode:           "en",
		VoiceSettings: &VoiceSettings{
			Stability:       &stability,
			SimilarityBoost: &similarity,
			Style:           &style,
			UseSpeakerBoost: &speakerBoost,
			Speed:           &speed,
		},
	}, 0)
	if err != nil {
		t.Fatalf("StreamTTS error: %v", err)
	}
}

func TestStreamTTS_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	_, err := c.ConvertTTS(context.Background(), "voice123", TTSRequest{Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}
