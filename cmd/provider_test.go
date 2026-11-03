package cmd

import (
	"strings"
	"testing"
)

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

func TestEnsureProviderConfigured(t *testing.T) {
	resetProviderEnv(t)
	if err := ensureProviderConfigured(); err == nil {
		t.Fatal("expected error with no keys")
	}
	t.Setenv("SIXTYDB_API_KEY", "sd-key")
	if err := ensureProviderConfigured(); err != nil {
		t.Fatalf("expected 60db key to satisfy configuration, got %v", err)
	}
}
