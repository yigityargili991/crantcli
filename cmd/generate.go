package cmd

import (
	"fmt"
	"os"

	"crantcli/internal/nglstate"

	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Output the default CRANT scene template",
	Long:  "Output the default CRANT Neuroglancer scene template to stdout or clipboard.",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprint(os.Stdout, string(nglstate.DefaultScene))
		return err
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
}
