package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderKeyFilePrecedence(t *testing.T) {
	for _, env := range []string{"SIXTYDB_API_KEY", "OPENAI_API_KEY", "DEEPGRAM_API_KEY"} {
		t.Run(env, func(t *testing.T) {
			resetProviderEnv(t)
			path := filepath.Join(t.TempDir(), "key")
			if err := os.WriteFile(path, []byte(" file-key\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(env+"_FILE", path)
			key, err := resolveProviderKey("test provider", env)
			if err != nil || key != "file-key" {
				t.Fatalf("file key: %v", err)
			}
			t.Setenv(env+"_FILE", path+"-missing")
			t.Setenv(env, " env-key ")
			key, err = resolveProviderKey("test provider", env)
			if err != nil || key != "env-key" {
				t.Fatalf("environment precedence: %v", err)
			}
			t.Setenv(env, "")
			if _, err = resolveProviderKey("test provider", env); err == nil {
				t.Fatal("missing key file accepted")
			}
			t.Setenv(env+"_FILE", path)
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = resolveProviderKey("test provider", env); err == nil {
				t.Fatal("empty key file accepted")
			}
		})
	}
}

// resetProviderEnv neutralizes every key source so each case starts clean.
// Setting an env var to "" makes the resolvers treat it as absent.
func resetProviderEnv(t *testing.T) {
	t.Helper()
	cfg.Provider = ""
	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	cfg.BaseURL = ""
	t.Cleanup(func() { cfg.Provider = ""; cfg.APIKey = ""; cfg.APIKeyFile = ""; cfg.BaseURL = "" })
	for _, k := range []string{
		"ELEVENLABS_API_KEY", "SAG_API_KEY",
		"ELEVENLABS_API_KEY_FILE", "SAG_API_KEY_FILE",
		"SIXTYDB_API_KEY", "SIXTYDB_API_KEY_FILE",
		"OPENAI_API_KEY", "OPENAI_API_KEY_FILE",
		"DEEPGRAM_API_KEY", "DEEPGRAM_API_KEY_FILE",
	} {
		t.Setenv(k, "")
	}
}

func TestSelectProvider_ElevenLabsOnly(t *testing.T) {
	resetProviderEnv(t)
	t.Setenv("ELEVENLABS_API_KEY", "el-key")

	provider, err := selectProvider()
	if err != nil {
		t.Fatalf("selectProvider error: %v", err)
	}
	if provider.name != providerElevenLabs || provider.elevenlabs == nil || provider.voices == nil {
		t.Fatalf("unexpected provider: %+v", provider)
	}
}

func TestSelectProvider_SixtyDBOnly(t *testing.T) {
	resetProviderEnv(t)
	t.Setenv("SIXTYDB_API_KEY", "sd-key")

	provider, err := selectProvider()
	if err != nil {
		t.Fatalf("selectProvider error: %v", err)
	}
	if provider.name != providerSixtyDB || provider.sixtydb == nil || provider.voices == nil {
		t.Fatalf("unexpected provider: %+v", provider)
	}
}

func TestSelectProvider_BothKeysError(t *testing.T) {
	resetProviderEnv(t)
	t.Setenv("ELEVENLABS_API_KEY", "el-key")
	t.Setenv("SIXTYDB_API_KEY", "sd-key")

	_, err := selectProvider()
	if err == nil || !strings.Contains(err.Error(), "ambiguous provider configuration") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestSelectProvider_NeitherErrors(t *testing.T) {
	resetProviderEnv(t)

	_, err := selectProvider()
	if err == nil {
		t.Fatal("expected error when no API key is set")
	}
}
