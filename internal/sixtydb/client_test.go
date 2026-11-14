package sixtydb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientDefaultsBase(t *testing.T) {
	c := NewClient("key", "")
	if c.baseURL != DefaultBaseURL {
		t.Fatalf("unexpected baseURL: %s", c.baseURL)
	}
}

func TestListVoicesMergesDefaultAndMyVoices(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		switch r.URL.Path {
		case "/default-voices":
			_, _ = io.WriteString(w, `{"success":true,"data":[
				{"voice_id":"v1","name":"Aria","category":"default","model":"60db Quality","labels":{"accent":"US"}},
				{"voice_id":"dup","name":"Default Dup","category":"default"}
			]}`)
		case "/myvoices":
			_, _ = io.WriteString(w, `{"success":true,"data":[
				{"voice_id":"dup","name":"My Dup","category":"cloned"},
				{"voice_id":"v2","name":"Ravi","category":"cloned","categories":["narration","warm"]}
			]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	voices, err := c.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices error: %v", err)
	}
	if want := []string{"/default-voices", "/myvoices"}; strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("route order = %v, want %v", calls, want)
	}
	if len(voices) != 3 {
		t.Fatalf("expected 3 merged voices, got %d", len(voices))
	}
	if voices[0].VoiceID != "v1" || voices[0].Labels["source"] != "default" || voices[0].Labels["model"] != "60db Quality" {
		t.Fatalf("unexpected default voice: %+v", voices[0])
	}
	if voices[1].VoiceID != "dup" || voices[1].Name != "Default Dup" {
		t.Fatalf("expected default duplicate to win, got %+v", voices[1])
	}
	if voices[2].VoiceID != "v2" || voices[2].Labels["source"] != "myvoices" || voices[2].Labels["categories"] != "narration, warm" {
		t.Fatalf("unexpected user voice: %+v", voices[2])
	}
}

func TestSearchVoicesFiltersAndLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/default-voices":
			_, _ = io.WriteString(w, `{"success":true,"data":[
				{"voice_id":"v1","name":"Roger"},
				{"voice_id":"v2","name":"Rogue"}
			]}`)
		case "/myvoices":
			_, _ = io.WriteString(w, `{"success":true,"data":[{"voice_id":"v3","name":"Sarah"}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	voices, err := c.SearchVoices(context.Background(), "rog", 1)
	if err != nil {
		t.Fatalf("SearchVoices error: %v", err)
	}
	if len(voices) != 1 || voices[0].VoiceID != "v1" {
		t.Fatalf("unexpected voices: %+v", voices)
	}
}

func TestGetVoiceUsesDocumentedPerVoiceRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/voices/voice-001" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{
			"id":"voice-001",
			"name":"Sarah",
			"description":"Professional female voice",
			"language":"en-US",
			"gender":"female",
			"age":"middle",
			"accent":"American",
			"use_case":["narration","customer-service"],
			"sample_url":"https://cdn.60db.com/samples/voice-001.mp3",
			"is_custom":false
		}`)
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	voice, err := c.GetVoice(context.Background(), "voice-001")
	if err != nil {
		t.Fatalf("GetVoice error: %v", err)
	}
	if voice.VoiceID != "voice-001" || voice.Name != "Sarah" {
		t.Fatalf("unexpected voice: %+v", voice)
	}
	if voice.PreviewURL != "https://cdn.60db.com/samples/voice-001.mp3" {
		t.Fatalf("unexpected preview URL: %q", voice.PreviewURL)
	}
	if voice.Labels["use_case"] != "narration, customer-service" || voice.Labels["accent"] != "American" {
		t.Fatalf("unexpected voice labels: %+v", voice.Labels)
	}
}

func TestListVoicesRejectsHTTP200ErrorEnvelopeWithoutLeakingToken(t *testing.T) {
	const secret = "secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":false,"message":"Bearer `+secret+` invalid token"}`)
	}))
	defer srv.Close()

	c := NewClient(secret, srv.URL)
	_, err := c.ListVoices(context.Background())
	assertSanitizedError(t, err, secret, "invalid token")
}

func TestConvertTTSUsesDocumentedRouteAndValidatesResponse(t *testing.T) {
	audio := []byte("ID3converted-audio")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tts-synthesize" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["text"] != "hi" || body["voice_id"] != "v1" {
			t.Fatalf("unexpected request body: %+v", body)
		}
		if body["speed"] != 1.1 || body["stability"] != 50.0 || body["similarity"] != 80.0 || body["output_format"] != "mp3" || body["sample_rate"] != float64(DefaultSampleRate) {
			t.Fatalf("unexpected mapped request body: %+v", body)
		}

		resp := map[string]any{
			"success":       true,
			"audio_base64":  base64.StdEncoding.EncodeToString(audio),
			"encoding":      "mp3",
			"output_format": "mp3",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	speed := 1.1
	stability := 50.0
	similarity := 80.0
	c := NewClient("key", srv.URL)
	got, err := c.ConvertTTS(context.Background(), TTSRequest{
		Text:         "hi",
		VoiceID:      "v1",
		Speed:        &speed,
		Stability:    &stability,
		Similarity:   &similarity,
		OutputFormat: "mp3_44100_128",
	})
	if err != nil {
		t.Fatalf("ConvertTTS error: %v", err)
	}
	if !bytes.Equal(got, audio) {
		t.Fatalf("decoded audio = %q, want %q", got, audio)
	}
}

func TestConvertTTSRejectsHTTP200ErrorEnvelopeWithoutLeakingToken(t *testing.T) {
	const secret = "secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":false,"message":"Bearer `+secret+` invalid token"}`)
	}))
	defer srv.Close()

	c := NewClient(secret, srv.URL)
	_, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi"})
	assertSanitizedError(t, err, secret, "invalid token")
}

func TestConvertTTSRejectsMalformedOrMismatchedAudio(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "empty audio",
			body: `{"success":true,"audio_base64":""}`,
			want: "empty audio_base64",
		},
		{
			name: "unknown format",
			body: `{"success":true,"audio_base64":"` + mustBase64([]byte("not-audio")) + `"}`,
			want: "unrecognized audio format",
		},
		{
			name: "declared format mismatch",
			body: `{"success":true,"audio_base64":"` + mustBase64([]byte("ID3mp3-data")) + `","output_format":"wav"}`,
			want: "declared wav, decoded mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			c := NewClient("key", srv.URL)
			_, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestConvertTTSAcceptsLiveNDJSONPCMAndWrapsWAV(t *testing.T) {
	chunk1 := []byte{0, 0, 1, 0}
	chunk2 := []byte{2, 0, 3, 0}
	inner, err := json.Marshal(map[string]any{
		"result": map[string]string{"audioContent": mustBase64(chunk2)},
	})
	if err != nil {
		t.Fatalf("marshal nested chunk: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tts-synthesize" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["output_format"] != "wav" || body["sample_rate"] != float64(DefaultSampleRate) {
			t.Fatalf("unexpected request body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"result":{"audioContent":"`+mustBase64(chunk1)+`"}}`+"\n")
		_, _ = io.WriteString(w, `{"result":{"audioContent":"`+mustBase64(inner)+`"}}`+"\n")
		_, _ = io.WriteString(w, `{"type":"meta","audio_sec":0.1}`+"\n")
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	got, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi", OutputFormat: "wav"})
	if err != nil {
		t.Fatalf("ConvertTTS error: %v", err)
	}
	if len(got) != 44+len(chunk1)+len(chunk2) {
		t.Fatalf("WAV length = %d, want %d", len(got), 44+len(chunk1)+len(chunk2))
	}
	if string(got[:4]) != "RIFF" || string(got[8:12]) != "WAVE" {
		t.Fatalf("missing WAV header: %q", got[:12])
	}
	wantPCM := append(append([]byte(nil), chunk1...), chunk2...)
	if !bytes.Equal(got[44:], wantPCM) {
		t.Fatalf("PCM body = %v, want %v", got[44:], wantPCM)
	}
}

func TestConvertTTSWrapsPCMThatStartsWithABrace(t *testing.T) {
	pcm := []byte{0x7b, 0x00, 0x01, 0x00}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"result":{"audioContent":"`+mustBase64(pcm)+`"}}`+"\n")
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	got, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi", OutputFormat: "wav"})
	if err != nil {
		t.Fatalf("ConvertTTS error: %v", err)
	}
	if len(got) != 44+len(pcm) || string(got[:4]) != "RIFF" || !bytes.Equal(got[44:], pcm) {
		t.Fatalf("WAV = %d bytes, prefix %q, pcm %v", len(got), got[:4], got[44:])
	}
}

func TestConvertTTSRejectsIncompleteLiveNDJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"result":{"audioContent":"`+mustBase64([]byte{0, 0})+`"}}`+"\n")
		_, _ = io.WriteString(w, `{"incomplete":true,"reasons":["truncated:ratio=0.48"]}`+"\n")
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	_, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi", OutputFormat: "wav"})
	if err == nil || !strings.Contains(err.Error(), "incomplete response: truncated:ratio=0.48") {
		t.Fatalf("expected incomplete response error, got %v", err)
	}
}

func TestConvertTTSRejectsLiveNDJSONErrorWithoutLeakingToken(t *testing.T) {
	const secret = "secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"success":false,"message":"Bearer `+secret+` invalid token"}`)
	}))
	defer srv.Close()

	c := NewClient(secret, srv.URL)
	_, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi", OutputFormat: "wav"})
	assertSanitizedError(t, err, secret, "invalid token")
}

func TestConvertTTSRejectsMalformedLiveNDJSON(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		format    string
		wantError string
	}{
		{
			name:      "bad json",
			body:      `not-json`,
			format:    "wav",
			wantError: "decode NDJSON frame",
		},
		{
			name:      "no audio",
			body:      `{"type":"meta"}`,
			format:    "wav",
			wantError: "contained no audio chunks",
		},
		{
			name:      "odd PCM byte count",
			body:      `{"result":{"audioContent":"` + mustBase64([]byte{1}) + `"}}`,
			format:    "wav",
			wantError: "incomplete 16-bit sample",
		},
		{
			name:      "raw PCM for mp3 request",
			body:      `{"result":{"audioContent":"` + mustBase64([]byte{0, 0}) + `"}}`,
			format:    "mp3",
			wantError: "raw PCM for requested mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			c := NewClient("key", srv.URL)
			_, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi", OutputFormat: tt.format})
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected %q, got %v", tt.wantError, err)
			}
		})
	}
}

func TestDecodeAudioContentRejectsPerChunkAndTotalLimits(t *testing.T) {
	tooLargeChunk := mustBase64(bytes.Repeat([]byte{'a'}, maxDecodedChunkBytes+1))
	if _, err := decodeAudioContent(tooLargeChunk, 0); err == nil || !strings.Contains(err.Error(), "audio chunk exceeds") {
		t.Fatalf("expected chunk limit error, got %v", err)
	}

	if _, err := decodeAudioContent(mustBase64([]byte("abc")), maxDecodedAudioBytes-2); err == nil || !strings.Contains(err.Error(), "audio exceeds") {
		t.Fatalf("expected total limit error, got %v", err)
	}
}

func TestStreamTTSUsesLiveSynthesizeRouteAndReturnsValidatedWAV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tts-synthesize" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"result":{"audioContent":"`+mustBase64([]byte{0, 0, 1, 0})+`"}}`)
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	rc, err := c.StreamTTS(context.Background(), TTSRequest{Text: "hi", OutputFormat: "wav"})
	if err != nil {
		t.Fatalf("StreamTTS error: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if string(got[:4]) != "RIFF" || string(got[8:12]) != "WAVE" {
		t.Fatalf("unexpected stream body: %q", got[:12])
	}
}

func TestCanonicalOutputFormat(t *testing.T) {
	cases := map[string]string{
		"mp3_44100_128": "mp3",
		"pcm_44100":     "wav",
		"wav":           "wav",
		"opus_48000_64": "ogg",
		"ogg":           "ogg",
		"flac":          "flac",
		"":              "",
	}
	for in, want := range cases {
		if got := CanonicalOutputFormat(in); got != want {
			t.Fatalf("CanonicalOutputFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

func mustBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func assertSanitizedError(t *testing.T, err error, secret, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked secret: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("expected redacted marker, got %v", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected %q in error, got %v", want, err)
	}
}

func TestConvertTTSWrapsPCMThatSharesAnMP3Sync(t *testing.T) {
	pcm := []byte{0xff, 0xfb, 0x00, 0x00}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"result":{"audioContent":"`+mustBase64(pcm)+`"}}`+"\n")
	}))
	defer srv.Close()

	c := NewClient("key", srv.URL)
	got, err := c.ConvertTTS(context.Background(), TTSRequest{Text: "hi", OutputFormat: "wav"})
	if err != nil {
		t.Fatalf("ConvertTTS error: %v", err)
	}
	if len(got) != 44+len(pcm) || string(got[:4]) != "RIFF" || !bytes.Equal(got[44:], pcm) {
		t.Fatalf("WAV = %d bytes, prefix %q, pcm %v", len(got), got[:4], got[44:])
	}
}

func TestSniffAudioFormatRequiresAUsableMP3Frame(t *testing.T) {
	if _, err := sniffAudioFormat([]byte{0xff, 0xfb, 0x00, 0x00}); err == nil {
		t.Fatal("expected PCM with a bare sync word to stay unrecognized")
	}
	got, err := sniffAudioFormat([]byte{0xff, 0xfb, 0x90, 0x00})
	if err != nil || got != audioFormatMP3 {
		t.Fatalf("valid frame = %q, %v", got, err)
	}
	got, err = sniffAudioFormat([]byte("ID3rest"))
	if err != nil || got != audioFormatMP3 {
		t.Fatalf("ID3 = %q, %v", got, err)
	}
}

func TestDecodeAudioContentRejectsMalformedNestedEnvelope(t *testing.T) {
	for _, input := range []string{`{}`, `{"result":"wrong type"}`, `{"audioContent":""}`} {
		if _, err := decodeAudioContent(mustBase64([]byte(input)), 0); err == nil {
			t.Errorf("accepted malformed envelope %s", input)
		}
	}
}
