package cmd

import (
	"errors"
	"fmt"
	"os"

	"crantinject/internal/config"

	"github.com/spf13/cobra"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "crantinject",
	Short:   "Query CRANT ant connectome neurons and inject into Neuroglancer states",
	Version: Version,
	Long: `crantinject queries the CRANT ant connectome dataset for neuron root IDs
by classification (super_class, cell_class, cell_type, cell_subtype) and
injects them into a Neuroglancer state JSON.

Run 'crantinject setup' to configure your SeaTable API token.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Annotations["requiresToken"] != "true" {
			return nil
		}
		if token := config.GetAPIToken(); token != "" {
			return nil
		}
		return config.RunSetupPrompt()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if !errors.Is(err, errStaleFound) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
