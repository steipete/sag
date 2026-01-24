package cmd

import (
	"os"
	"testing"
)

// keepEnv saves and restores env vars used by ensureAPIKey.
func keepEnv(t *testing.T) func() {
	t.Helper()
	orig := map[string]string{
		"MINIMAX_API_KEY":         os.Getenv("MINIMAX_API_KEY"),
		"SAG_API_KEY":             os.Getenv("SAG_API_KEY"),
		"MINIMAX_API_KEY_FILE":    os.Getenv("MINIMAX_API_KEY_FILE"),
		"SAG_API_KEY_FILE":        os.Getenv("SAG_API_KEY_FILE"),
	}
	return func() {
		_ = os.Setenv("MINIMAX_API_KEY", orig["MINIMAX_API_KEY"])
		_ = os.Setenv("SAG_API_KEY", orig["SAG_API_KEY"])
		_ = os.Setenv("MINIMAX_API_KEY_FILE", orig["MINIMAX_API_KEY_FILE"])
		_ = os.Setenv("SAG_API_KEY_FILE", orig["SAG_API_KEY_FILE"])
	}
}

func TestEnsureAPIKeyPrefersCLIValue(t *testing.T) {
	defer keepEnv(t)()
	cfg.APIKey = "cli-key"
	cfg.APIKeyFile = ""
	_ = os.Unsetenv("MINIMAX_API_KEY")
	_ = os.Unsetenv("SAG_API_KEY")
	_ = os.Unsetenv("MINIMAX_API_KEY_FILE")
	_ = os.Unsetenv("SAG_API_KEY_FILE")

	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "cli-key" {
		t.Fatalf("expected CLI cfg API key to win, got %q", cfg.APIKey)
	}
}

func TestEnsureAPIKeyFromFileFlag(t *testing.T) {
	defer keepEnv(t)()
	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	_ = os.Unsetenv("MINIMAX_API_KEY")
	_ = os.Unsetenv("SAG_API_KEY")
	_ = os.Unsetenv("MINIMAX_API_KEY_FILE")
	_ = os.Unsetenv("SAG_API_KEY_FILE")

	tmp, err := os.CreateTemp("", "sag_api_key")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString("file-key\n"); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}

	cfg.APIKeyFile = tmp.Name()
	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "file-key" {
		t.Fatalf("expected file key to be used, got %q", cfg.APIKey)
	}
}

func TestEnsureAPIKeyFromEnvFileOrder(t *testing.T) {
	defer keepEnv(t)()
	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	_ = os.Unsetenv("MINIMAX_API_KEY")
	_ = os.Unsetenv("SAG_API_KEY")

	tmpPrimary, err := os.CreateTemp("", "sag_api_key_primary")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpPrimary.Name()) }()
	if _, err := tmpPrimary.WriteString("primary-key"); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := tmpPrimary.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}

	tmpFallback, err := os.CreateTemp("", "sag_api_key_fallback")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFallback.Name()) }()
	if _, err := tmpFallback.WriteString("fallback-key"); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := tmpFallback.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}

	_ = os.Setenv("MINIMAX_API_KEY_FILE", tmpPrimary.Name())
	_ = os.Setenv("SAG_API_KEY_FILE", tmpFallback.Name())
	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "primary-key" {
		t.Fatalf("expected MINIMAX_API_KEY_FILE to be used, got %q", cfg.APIKey)
	}

	cfg.APIKey = ""
	_ = os.Unsetenv("MINIMAX_API_KEY_FILE")
	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "fallback-key" {
		t.Fatalf("expected SAG_API_KEY_FILE to be used, got %q", cfg.APIKey)
	}
}

func TestEnsureAPIKeyFallsBackToEnvOrder(t *testing.T) {
	defer keepEnv(t)()
	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	_ = os.Setenv("MINIMAX_API_KEY", "env-key")
	_ = os.Setenv("SAG_API_KEY", "sag-key")
	_ = os.Unsetenv("MINIMAX_API_KEY_FILE")
	_ = os.Unsetenv("SAG_API_KEY_FILE")

	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "env-key" {
		t.Fatalf("expected MINIMAX_API_KEY to be used, got %q", cfg.APIKey)
	}

	// Clear primary env to ensure SAG_API_KEY is used next.
	cfg.APIKey = ""
	_ = os.Unsetenv("MINIMAX_API_KEY")
	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "sag-key" {
		t.Fatalf("expected SAG_API_KEY to be used, got %q", cfg.APIKey)
	}
}

func TestEnsureAPIKeyMissing(t *testing.T) {
	defer keepEnv(t)()
	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	_ = os.Unsetenv("MINIMAX_API_KEY")
	_ = os.Unsetenv("SAG_API_KEY")
	_ = os.Unsetenv("MINIMAX_API_KEY_FILE")
	_ = os.Unsetenv("SAG_API_KEY_FILE")

	if err := ensureAPIKey(); err == nil {
		t.Fatal("expected error when API key missing")
	}
}
