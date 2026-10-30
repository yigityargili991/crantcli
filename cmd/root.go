// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"os"

	"crantcli/internal/config"
	"crantcli/internal/nglstate"
	"crantcli/internal/statesrv"

	"github.com/spf13/cobra"
)

var Version = "dev"

var getAPIToken = config.GetAPIToken

// errNoSeaTableToken is the canonical missing-token error. Commands declare
// their requirement statically (the requiresToken annotation) or from their
// flags, but a missing token always reads the same way.
var errNoSeaTableToken = errors.New("no SeaTable token configured; run 'crantcli setup' to store one, or set CRANTTABLE_TOKEN / CRANTTABLE_TOKEN_FILE")

// requireSeaTableToken reports the canonical missing-token error when the run
// needs SeaTable (needed) and no token is configured. Commands whose token
// requirement depends on their flags share the annotation-gated path's message.
func requireSeaTableToken(needed bool) error {
	if !needed || getAPIToken() != "" {
		return nil
	}
	return errNoSeaTableToken
}

// stateServerResolver fetches the states Neuroglancer Share links point to.
// Each token goes only to the host of the auth server that issued it, whose
// state server accepts it, on the default https port: the CAVE token to
// proofreading.zetta.ai, the global one to global.daf-apis.com. The CAVE
// server, data.proofreading.zetta.ai, has no state server, so this resolver
// sends it no token.
var stateServerResolver = &statesrv.Resolver{
	Credentials: []statesrv.Credential{
		{
			Name:       "CAVE token",
			AuthServer: config.CAVEAuthServer,
			Token:      config.LookupCAVEToken,
			Sources:    config.CAVETokenSources(),
			Setup:      "run 'crantcli setup' or set CAVE_TOKEN",
		},
		{
			Name:       "global CAVE token",
			AuthServer: config.CAVEGlobalAuthServer,
			Token:      config.LookupCAVEGlobalToken,
			Sources:    config.CAVEGlobalTokenSources(),
			Setup:      "run 'crantcli setup' or set CAVE_GLOBAL_TOKEN",
		},
	},
}

func init() {
	// Installed once, so every command that loads a state can read a Share
	// link: from the clipboard (add, inspect), --state, and check-cave.
	nglstate.SetRemoteStateResolver(func(remote nglstate.RemoteState) ([]byte, error) {
		return stateServerResolver.Resolve(remote.Location)
	})
}

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
		// Do not silently write credentials from a read-only query command:
		// point the user at the explicit setup flow instead of prompting here.
		return requireSeaTableToken(cmd.Annotations["requiresToken"] == "true")
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
