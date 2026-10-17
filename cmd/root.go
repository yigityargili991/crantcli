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
		// Cobra validates arguments before this hook. Keep usage for syntax
		// mistakes, but suppress it once execution has entered the runtime path.
		cmd.Root().SilenceUsage = true
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
	cmd, err := rootCmd.ExecuteC()
	if err != nil && !errors.Is(err, errStaleFound) {
		fmt.Fprintln(os.Stderr, err)
	}
	if code := exitCode(cmd, err); code != 0 {
		os.Exit(code)
	}
}

// exitCode maps a command's result to the process exit status. check-cave
// --quiet answers "stale root IDs found" with 1, so check-cave failures use 2
// to stay distinguishable from that answer.
func exitCode(cmd *cobra.Command, err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errStaleFound):
		return 1
	case cmd == checkCaveCmd:
		return 2
	default:
		return 1
	}
}
