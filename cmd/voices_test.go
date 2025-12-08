package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestVoicesCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/voices" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"voices":[{"voice_id":"id1","name":"Alpha","category":"premade"}]}`))
	}))
	defer srv.Close()

	cfg.APIKey = "key"
	cfg.BaseURL = srv.URL

	restore, readOut := captureStdoutVoices(t)
	defer restore()

	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"voices", "--limit", "1"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd execute: %v", err)
	}

	out := buf.String() + readOut()
	if !bytes.Contains([]byte(out), []byte("VOICE ID")) {
		t.Fatalf("expected table output, got %q", out)
	}

	// reset args to avoid polluting other tests
	rootCmd.SetArgs(nil)
	_ = os.Unsetenv("ELEVENLABS_API_KEY")
}

func captureStdoutVoices(t *testing.T) (restore func(), read func() string) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	return func() {
			_ = w.Close()
			os.Stdout = orig
		}, func() string {
			_ = w.Close()
			b, _ := io.ReadAll(r)
			return string(b)
		}
}
