package openai

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func testServer(t *testing.T, events []any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/realtime" || r.URL.Query().Get("model") != DefaultModel || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := wsjson.Write(ctx, conn, map[string]any{"type": "session.created"}); err != nil {
			t.Error(err)
			return
		}
		var update struct {
			Type    string `json:"type"`
			Session struct {
				Type  string `json:"type"`
				Audio struct {
					Output struct {
						Voice  string  `json:"voice"`
						Speed  float64 `json:"speed"`
						Format struct {
							Type string `json:"type"`
							Rate int    `json:"rate"`
						} `json:"format"`
					} `json:"output"`
				} `json:"audio"`
			} `json:"session"`
		}
		if err := wsjson.Read(ctx, conn, &update); err != nil {
			t.Error(err)
			return
		}
		out := update.Session.Audio.Output
		if update.Type != "session.update" || update.Session.Type != "realtime" || out.Voice != "marin" || out.Speed != 1.2 || out.Format.Type != "audio/pcm" || out.Format.Rate != 24000 {
			t.Errorf("unexpected session update: %+v", update)
		}
		if err := wsjson.Write(ctx, conn, map[string]any{"type": "session.updated"}); err != nil {
			t.Error(err)
			return
		}
		var create struct {
			Type     string `json:"type"`
			Response struct {
				Conversation string `json:"conversation"`
				Instructions string `json:"instructions"`
				Input        []struct {
					Role    string `json:"role"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"input"`
			} `json:"response"`
		}
		if err := wsjson.Read(ctx, conn, &create); err != nil {
			t.Error(err)
			return
		}
		if create.Type != "response.create" || create.Response.Conversation != "none" || !strings.Contains(create.Response.Instructions, "verbatim") || len(create.Response.Input) != 1 || len(create.Response.Input[0].Content) != 1 || create.Response.Input[0].Content[0].Text != "Hello world" {
			t.Errorf("unexpected response request: %+v", create)
		}
		for _, event := range events {
			if err := wsjson.Write(ctx, conn, event); err != nil {
				return
			}
		}
	}))
}

func audioDelta(data []byte) any {
	return map[string]any{"type": "response.output_audio.delta", "delta": base64.StdEncoding.EncodeToString(data)}
}

func responseDone(status string) any {
	return map[string]any{"type": "response.done", "response": map[string]any{"status": status}}
}

func TestConvertTTS(t *testing.T) {
	audioDone := map[string]any{"type": "response.output_audio.done"}
	for _, tc := range []struct {
		name      string
		events    []any
		wantError string
	}{
		{"complete", []any{map[string]any{"type": "response.created"}, audioDelta([]byte{1}), audioDelta([]byte{2, 3, 4}), audioDone, responseDone("completed")}, ""},
		{"disconnect", []any{audioDelta([]byte{1, 2})}, "incomplete"},
		{"failed", []any{audioDelta([]byte{1, 2}), audioDone, responseDone("failed")}, "did not complete"},
		{"incomplete status", []any{audioDelta([]byte{1, 2}), audioDone, responseDone("incomplete")}, "did not complete"},
		{"no audio done", []any{audioDelta([]byte{1, 2}), responseDone("completed")}, "incomplete"},
		{"empty", []any{audioDone, responseDone("completed")}, "missing"},
		{"unaligned", []any{audioDelta([]byte{1}), audioDone, responseDone("completed")}, "unaligned"},
		{"invalid base64", []any{map[string]any{"type": "response.output_audio.delta", "delta": "%%%"}}, "base64"},
		{"late audio", []any{audioDone, audioDelta([]byte{1, 2})}, "unexpected audio"},
		{"api error", []any{map[string]any{"type": "error", "error": map[string]any{"message": "bad test-key"}}}, "bad [redacted]"},
		{"malformed event", []any{json.RawMessage(`{"type":123}`)}, "incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServer(t, tc.events)
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			data, err := NewClient("test-key", srv.URL+"/v1").ConvertTTS(ctx, TTSRequest{Text: "Hello world", Speed: 1.2})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || len(data) != 0 {
					t.Fatalf("data=%d err=%v; want %s", len(data), err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(data) != 48 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || binary.LittleEndian.Uint32(data[24:]) != 24000 || binary.LittleEndian.Uint32(data[40:]) != 4 || string(data[44:]) != "\x01\x02\x03\x04" {
				t.Fatalf("invalid WAV: %v", data)
			}
		})
	}
}

func TestConvertTTSCancellation(t *testing.T) {
	connected := make(chan struct{})
	disconnected := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow(); close(disconnected) }()
		close(connected)
		_, _, _ = conn.Read(r.Context())
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := NewClient("test-key", srv.URL).ConvertTTS(ctx, TTSRequest{Text: "Hello world", Speed: 1})
		result <- err
	}()
	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("did not connect")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop read")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("connection was not closed")
	}
}

func TestVoicesAndEndpoint(t *testing.T) {
	c := NewClient("test-key", "")
	voices, _ := c.ListVoices(context.Background())
	if len(voices) != 10 || voices[0].VoiceID != "marin" {
		t.Fatalf("voices: %v", voices)
	}
	voices, _ = c.SearchVoices(context.Background(), "AR", 1)
	if len(voices) != 1 || voices[0].VoiceID != "marin" {
		t.Fatalf("search: %v", voices)
	}
	if _, err := c.GetVoice(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown voice accepted")
	}
	u, err := c.endpoint(DefaultModel)
	if err != nil || u != "wss://api.openai.com/v1/realtime?model="+DefaultModel {
		t.Fatalf("endpoint=%s err=%v", u, err)
	}
}

func TestConvertTTSValidation(t *testing.T) {
	for _, req := range []TTSRequest{
		{Text: "", Speed: 1}, {Text: "hello", Speed: 2}, {Text: "hello", Speed: math.NaN()}, {Text: "hello", Speed: 1, VoiceID: "unknown"},
	} {
		if _, err := NewClient("test-key", "http://unused.invalid").ConvertTTS(context.Background(), req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
	for _, base := range []string{"file:///tmp", "https:///missing", "https://user:pass@example.com/v1", "https://example.com/v1?key=bad"} {
		if _, err := NewClient("test-key", base).endpoint(DefaultModel); err == nil {
			t.Errorf("accepted %s", base)
		}
	}
}

func TestConvertTTSAudioLimits(t *testing.T) {
	chunk := audioDelta(make([]byte, 1<<20))
	var tooMuchAudio []any
	for i := 0; i <= maxAudioBytes/(1<<20); i++ {
		tooMuchAudio = append(tooMuchAudio, chunk)
	}
	for _, tc := range []struct {
		name   string
		events []any
		want   string
	}{
		{"event limit", []any{map[string]any{"type": "response.output_audio.delta", "delta": strings.Repeat("A", maxEventBytes)}}, "incomplete"},
		{"total audio limit", tooMuchAudio, "64 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServer(t, tc.events)
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			data, err := NewClient("test-key", srv.URL+"/v1").ConvertTTS(ctx, TTSRequest{Text: "Hello world", Speed: 1.2})
			if len(data) != 0 || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("data=%d err=%v", len(data), err)
			}
		})
	}
}

func TestConvertTTSHandshakeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid test-key", http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := NewClient("test-key", srv.URL).ConvertTTS(context.Background(), TTSRequest{Text: "hello", Speed: 1})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("unexpected handshake error: %v", err)
	}
}
