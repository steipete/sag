package cmd

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestGetAPIKeyFromKeychain(t *testing.T) {
	keyring.MockInit()

	// Test empty keychain returns empty string
	key := getAPIKeyFromKeychain()
	if key != "" {
		t.Fatalf("expected empty string for empty keychain, got %q", key)
	}

	// Store a key and verify it can be retrieved
	testKey := "test-api-key-12345"
	if err := keyring.Set(keychainService, keychainUser, testKey); err != nil {
		t.Fatalf("failed to set keychain key: %v", err)
	}

	key = getAPIKeyFromKeychain()
	if key != testKey {
		t.Fatalf("expected %q, got %q", testKey, key)
	}

	// Clean up
	_ = keyring.Delete(keychainService, keychainUser)
}

func TestEnsureAPIKeyFromKeychain(t *testing.T) {
	keyring.MockInit()
	defer keepEnv(t)()

	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	_ = os.Unsetenv("ELEVENLABS_API_KEY")
	_ = os.Unsetenv("SAG_API_KEY")
	_ = os.Unsetenv("ELEVENLABS_API_KEY_FILE")
	_ = os.Unsetenv("SAG_API_KEY_FILE")

	// Store key in mock keychain
	testKey := "keychain-api-key"
	if err := keyring.Set(keychainService, keychainUser, testKey); err != nil {
		t.Fatalf("failed to set keychain key: %v", err)
	}

	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != testKey {
		t.Fatalf("expected keychain key to be used, got %q", cfg.APIKey)
	}

	// Clean up
	_ = keyring.Delete(keychainService, keychainUser)
}

func TestKeychainPriorityAfterFile(t *testing.T) {
	keyring.MockInit()
	defer keepEnv(t)()

	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	_ = os.Unsetenv("ELEVENLABS_API_KEY")
	_ = os.Unsetenv("SAG_API_KEY")
	_ = os.Unsetenv("ELEVENLABS_API_KEY_FILE")
	_ = os.Unsetenv("SAG_API_KEY_FILE")

	// Create temp file with API key
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

	// Store key in mock keychain
	if err := keyring.Set(keychainService, keychainUser, "keychain-key"); err != nil {
		t.Fatalf("failed to set keychain key: %v", err)
	}

	// File should take priority over keychain
	cfg.APIKeyFile = tmp.Name()
	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "file-key" {
		t.Fatalf("expected file key to take priority, got %q", cfg.APIKey)
	}

	// Clean up
	_ = keyring.Delete(keychainService, keychainUser)
}

func TestKeychainPriorityOverEnv(t *testing.T) {
	keyring.MockInit()
	defer keepEnv(t)()

	cfg.APIKey = ""
	cfg.APIKeyFile = ""
	_ = os.Setenv("ELEVENLABS_API_KEY", "env-key")
	_ = os.Unsetenv("SAG_API_KEY")
	_ = os.Unsetenv("ELEVENLABS_API_KEY_FILE")
	_ = os.Unsetenv("SAG_API_KEY_FILE")

	// Store key in mock keychain
	if err := keyring.Set(keychainService, keychainUser, "keychain-key"); err != nil {
		t.Fatalf("failed to set keychain key: %v", err)
	}

	// Keychain should take priority over env
	if err := ensureAPIKey(); err != nil {
		t.Fatalf("ensureAPIKey error: %v", err)
	}
	if cfg.APIKey != "keychain-key" {
		t.Fatalf("expected keychain key to take priority over env, got %q", cfg.APIKey)
	}

	// Clean up
	_ = keyring.Delete(keychainService, keychainUser)
}
