package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/steipete/sag/internal/chatgpt"
	"github.com/steipete/sag/internal/deepgram"
	"github.com/steipete/sag/internal/elevenlabs"
	"github.com/steipete/sag/internal/openai"
	"github.com/steipete/sag/internal/sixtydb"
	"github.com/steipete/sag/internal/tts"
)

const (
	providerElevenLabs = "elevenlabs"
	providerSixtyDB    = "60db"
	providerOpenAI     = "openai"
	providerChatGPT    = "chatgpt"
	providerDeepgram   = "deepgram"
)

type activeProvider struct {
	name   string
	voices tts.VoiceCatalog

	elevenlabs *elevenlabs.Client
	sixtydb    *sixtydb.Client
	openai     *openai.Client
	chatgpt    *chatgpt.Client
	deepgram   *deepgram.Client
}

// resolveSixtyDBKey resolves the 60db API key from its dedicated env vars.
// Order: SIXTYDB_API_KEY, then SIXTYDB_API_KEY_FILE.
func resolveSixtyDBKey() (string, error) {
	if key := strings.TrimSpace(os.Getenv("SIXTYDB_API_KEY")); key != "" {
		return key, nil
	}
	if path := strings.TrimSpace(os.Getenv("SIXTYDB_API_KEY_FILE")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read 60db api key file: %w", err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return "", fmt.Errorf("60db api key file %q is empty", path)
		}
		return key, nil
	}
	return "", nil
}

func ensureProviderConfigured() error {
	_, err := selectProvider()
	return err
}

func resolveOpenAIKey() (string, error) {
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		return key, nil
	}
	if path := strings.TrimSpace(os.Getenv("OPENAI_API_KEY_FILE")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read OpenAI api key file: %w", err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return "", fmt.Errorf("OpenAI api key file %q is empty", path)
		}
		return key, nil
	}
	return "", nil
}

func selectProvider() (activeProvider, error) {
	selected := strings.ToLower(strings.TrimSpace(cfg.Provider))
	// ChatGPT is always explicit; signing in must not alter existing defaults.
	if selected == providerChatGPT {
		if cfg.APIKey != "" || cfg.APIKeyFile != "" || cfg.BaseURL != "" {
			return activeProvider{}, fmt.Errorf("ChatGPT uses its separate account login; --api-key, --api-key-file, and --base-url are not supported")
		}
		client, err := chatgpt.NewClient()
		return activeProvider{name: providerChatGPT, voices: client, chatgpt: client}, err
	}
	// New providers are explicit so an ambient key cannot change existing commands.
	if selected == providerDeepgram {
		key, err := resolveDeepgramKey()
		if err != nil {
			return activeProvider{}, err
		}
		if key == "" {
			return activeProvider{}, fmt.Errorf("missing API key for Deepgram (set DEEPGRAM_API_KEY or DEEPGRAM_API_KEY_FILE)")
		}
		client := deepgram.NewClient(key, cfg.BaseURL)
		return activeProvider{name: providerDeepgram, voices: client, deepgram: client}, nil
	}
	if selected != "" && selected != providerElevenLabs && selected != providerSixtyDB && selected != providerOpenAI {
		return activeProvider{}, fmt.Errorf("unknown provider %q (choose elevenlabs, 60db, openai, chatgpt, or deepgram)", selected)
	}
	var active activeProvider
	for _, candidate := range []struct {
		name string
		key  func() (string, error)
	}{
		{providerElevenLabs, resolveElevenLabsKey},
		{providerSixtyDB, resolveSixtyDBKey},
		{providerOpenAI, resolveOpenAIKey},
	} {
		if selected != "" && selected != candidate.name {
			continue
		}
		// An ambient OpenAI key must not change existing ElevenLabs/60db commands.
		if selected == "" && candidate.name == providerOpenAI && active.name != "" {
			continue
		}
		key, err := candidate.key()
		if err != nil {
			return activeProvider{}, err
		}
		if key == "" {
			continue
		}
		if active.name != "" {
			return activeProvider{}, fmt.Errorf("ambiguous provider configuration: multiple provider keys are set; choose --provider elevenlabs, 60db, or openai")
		}
		active.name = candidate.name
		switch candidate.name {
		case providerElevenLabs:
			active.elevenlabs = elevenlabs.NewClient(key, cfg.BaseURL)
			active.voices = active.elevenlabs
		case providerSixtyDB:
			active.sixtydb = sixtydb.NewClient(key, cfg.BaseURL)
			active.voices = active.sixtydb
		case providerOpenAI:
			active.openai = openai.NewClient(key, cfg.BaseURL)
			active.voices = active.openai
		}
	}
	if active.name == "" {
		if selected != "" {
			return activeProvider{}, fmt.Errorf("missing API key for %s", selected)
		}
		return activeProvider{}, fmt.Errorf("missing API key (set ELEVENLABS_API_KEY, SIXTYDB_API_KEY, or OPENAI_API_KEY)")
	}
	return active, nil
}

var sixtyDBUnsupportedFlags = []string{
	"model-id",
	"style",
	"speaker-boost",
	"no-speaker-boost",
	"seed",
	"normalize",
	"lang",
	"latency-tier",
}

func changedSixtyDBUnsupportedFlags(changed func(string) bool) []string {
	var unsupported []string
	for _, name := range sixtyDBUnsupportedFlags {
		if changed(name) {
			unsupported = append(unsupported, "--"+name)
		}
	}
	return unsupported
}
