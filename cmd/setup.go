package cmd

import (
	"crantcli/internal/config"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set or update API tokens (SeaTable and CAVE)",
	Long: `Interactively set or update the SeaTable API token and optional CAVE token.

Tokens are stored in ~/.crantcli/ as base64-encoded files with 0600 permissions
(directory 0700). Note that base64 is obfuscation, not encryption: the file
permissions are what protect the token, and crantcli tightens them back to
0600 if it finds them looser.`,
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
