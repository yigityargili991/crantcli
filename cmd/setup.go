package cmd

import (
	"crantcli/internal/config"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set or update API tokens (SeaTable and CAVE)",
	Long: `Interactively set or update the SeaTable API token and optional CAVE token.

Tokens are stored in the operating system's secure credential manager: Keychain
on macOS, Credential Manager on Windows, and Secret Service on Linux. If Secret
Service is unavailable on Linux, crantcli uses an owner-only file in ~/.crantcli/.
Existing file-based credentials are migrated automatically when possible.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.RunSetupPrompt(); err != nil {
			return err
		}
		return config.RunCAVESetupPrompt()
	},
}

func init() {
	setupCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(setupCmd)
}
