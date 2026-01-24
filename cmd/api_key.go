package cmd

import (
	"fmt"
	"os"
	"strings"
)

func ensureAPIKey() error {
	if cfg.APIKey == "" {
		key, err := resolveAPIKeyFromFile()
		if err != nil {
			return err
		}
		cfg.APIKey = key
	}
	if cfg.APIKey == "" {
		cfg.APIKey = getAPIKeyFromKeychain()
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("ELEVENLABS_API_KEY")
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("SAG_API_KEY")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("missing ElevenLabs API key (set --api-key, --api-key-file, keychain, or ELEVENLABS_API_KEY)")
	}
	return nil
}

func resolveAPIKeyFromFile() (string, error) {
	path := cfg.APIKeyFile
	if path == "" {
		path = os.Getenv("ELEVENLABS_API_KEY_FILE")
	}
	if path == "" {
		path = os.Getenv("SAG_API_KEY_FILE")
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read api key file: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("api key file %q is empty", path)
	}
	return key, nil
}
