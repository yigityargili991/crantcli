package cmd

import (
	"crant_type_look/internal/config"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set or update the SeaTable API token",
	Long:  "Interactively set or update the SeaTable API token stored in ~/.crant_type_look/credentials.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return config.RunSetupPrompt()
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
