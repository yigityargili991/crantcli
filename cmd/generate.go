// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"crantcli/internal/nglstate"

	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:        "generate",
	Short:      "Output the default CRANT scene template",
	Long:       "Output the default CRANT Neuroglancer scene template to stdout.",
	Hidden:     true,
	Deprecated: "use 'change-def-state --builtin' instead; generate will be removed in v0.25.0",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprint(os.Stdout, string(nglstate.DefaultScene))
		return err
	},
}

func init() {
	generateCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(generateCmd)
}
