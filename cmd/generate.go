package cmd

import (
	"fmt"

	"crantcli/internal/nglstate"

	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Output the default CRANT scene template",
	Long:  "Output the default CRANT Neuroglancer scene template to stdout.",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprint(cmd.OutOrStdout(), string(nglstate.DefaultScene))
		return err
	},
}

func init() {
	generateCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(generateCmd)
}
