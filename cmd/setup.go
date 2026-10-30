// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"crantcli/internal/config"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set or update API tokens (SeaTable and CAVE)",
	Long: `Interactively set or update the SeaTable API token, the optional CAVE token,
and the optional global CAVE token, then offer to download the model for the
experimental crantcli ask command.

The CAVE token, issued by proofreading.zetta.ai, serves the CAVE commands and
Neuroglancer Share links stored on proofreading.zetta.ai. The global CAVE token,
issued by global.daf-apis.com, is sent only there, for Share links stored on
global.daf-apis.com. Without one, crantcli reads the token caveclient saved in
~/.cloudvolume/secrets/global.daf-apis.com-cave-secret.json, a file it never
writes.

Tokens are stored in the operating system's secure credential manager: Keychain
on macOS, Credential Manager on Windows, and Secret Service on Linux. If Secret
Service is unavailable on Linux, crantcli uses an owner-only file in ~/.crantcli/.
Existing file-based credentials are migrated automatically when possible.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.RunSetupPrompt(); err != nil {
			return err
		}
		if err := config.RunCAVESetupPrompt(); err != nil {
			return err
		}
		if err := config.RunCAVEGlobalSetupPrompt(); err != nil {
			return err
		}
		return offerAskModel(cmd)
	},
}

func init() {
	setupCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(setupCmd)
}
