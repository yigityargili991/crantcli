package cmd

import (
	"crantinject/internal/config"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set or update API tokens (SeaTable and CAVE)",
	Long:  "Interactively set or update the SeaTable API token and optional CAVE token stored in ~/.crantcli/.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.RunSetupPrompt(); err != nil {
			return err
		}
		return config.RunCAVESetupPrompt()
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
