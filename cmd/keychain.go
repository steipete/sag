package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

const (
	keychainService = "sag"
	keychainUser    = "elevenlabs-api-key"
)

var keychainCmd = &cobra.Command{
	Use:   "keychain",
	Short: "Manage API key in system keychain",
	Long:  "Store, retrieve, or delete the ElevenLabs API key from the system keychain.\n\nSupported backends:\n  - macOS: Keychain\n  - Windows: Credential Manager\n  - Linux: Secret Service (GNOME Keyring)",
}

var keychainSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Store API key in system keychain",
	Long:  "Store the ElevenLabs API key securely in the system keychain.\nYou will be prompted to enter the key (input is hidden).",
	RunE: func(_ *cobra.Command, args []string) error {
		var apiKey string
		if len(args) > 0 {
			apiKey = args[0]
		} else {
			fmt.Print("Enter ElevenLabs API key: ")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			apiKey = strings.TrimSpace(input)
		}

		if apiKey == "" {
			return fmt.Errorf("API key cannot be empty")
		}

		if err := keyring.Set(keychainService, keychainUser, apiKey); err != nil {
			return fmt.Errorf("failed to store API key in keychain: %w", err)
		}

		fmt.Println("API key stored in system keychain")
		return nil
	},
}

var keychainGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Retrieve API key from system keychain",
	Long:  "Retrieve and display the ElevenLabs API key from the system keychain.",
	RunE: func(_ *cobra.Command, _ []string) error {
		secret, err := keyring.Get(keychainService, keychainUser)
		if err != nil {
			if err == keyring.ErrNotFound {
				return fmt.Errorf("no API key found in keychain (use 'sag keychain set' to store one)")
			}
			return fmt.Errorf("failed to retrieve API key from keychain: %w", err)
		}

		fmt.Println(secret)
		return nil
	},
}

var keychainDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete API key from system keychain",
	Long:  "Remove the ElevenLabs API key from the system keychain.",
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := keyring.Delete(keychainService, keychainUser); err != nil {
			if err == keyring.ErrNotFound {
				return fmt.Errorf("no API key found in keychain")
			}
			return fmt.Errorf("failed to delete API key from keychain: %w", err)
		}

		fmt.Println("API key deleted from system keychain")
		return nil
	},
}

func init() {
	keychainCmd.AddCommand(keychainSetCmd)
	keychainCmd.AddCommand(keychainGetCmd)
	keychainCmd.AddCommand(keychainDeleteCmd)
	rootCmd.AddCommand(keychainCmd)
}

// getAPIKeyFromKeychain attempts to retrieve the API key from the system keychain.
// Returns empty string if not found or on error.
func getAPIKeyFromKeychain() string {
	secret, err := keyring.Get(keychainService, keychainUser)
	if err != nil {
		return ""
	}
	return secret
}
