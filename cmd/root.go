package cmd

import (
	"errors"
	"fmt"
	"os"

	"crantcli/internal/config"

	"github.com/spf13/cobra"
)

var Version = "dev"

var getAPIToken = config.GetAPIToken

var rootCmd = &cobra.Command{
	Use:           "crantcli",
	Short:         "Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states",
	Version:       Version,
	SilenceErrors: true,
	Long: `crantcli queries the CRANT (Clonal Raider Ant Connectome) dataset for neuron root IDs
by classification (super_class, cell_class, cell_type, cell_subtype) and
injects them into a Neuroglancer state JSON.

Run 'crantcli setup' to configure your SeaTable API token.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Annotations["requiresToken"] != "true" {
			return nil
		}
		if token := getAPIToken(); token != "" {
			return nil
		}
		// Do not silently write credentials from a read-only query command:
		// point the user at the explicit setup flow instead of prompting here.
		return errors.New("no SeaTable token configured; run 'crantcli setup' to store one, or set CRANTTABLE_TOKEN / CRANTTABLE_TOKEN_FILE")
	},
}

// RootCommand exposes the configured command tree to documentation tooling.
// Application code should continue to call Execute.
func RootCommand() *cobra.Command {
	return rootCmd
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if !errors.Is(err, errStaleFound) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
