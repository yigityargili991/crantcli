// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"crantcli/internal/textout"

	"github.com/spf13/cobra"
)

var lookupColumnCmd = &cobra.Command{
	Use:         "lookup-column [root_id]",
	Short:       "Find the closest EPG/PEG neuron's column (region) by position",
	Long:        `Find the closest EPG/PEG neuron to the given root ID (or position) by 3D Euclidean distance and print its resolved region value plus the nearest EPG/PEG root ID.`,
	Annotations: map[string]string{"requiresToken": "true"},
	Args:        cobra.MaximumNArgs(1),
	Hidden:      true,
	Deprecated:  "use 'root-info' with a root ID or --pos x,y,z instead; lookup-column will be removed in v0.25.0",
}

func init() {
	var lookupColumnPos string

	lookupColumnCmd.Flags().StringVar(&lookupColumnPos, "pos", "", "Position as x,y,z (e.g. 100.5,200.3,300.1)")
	lookupColumnCmd.ValidArgsFunction = noFileCompletion
	mustRegisterFlagCompletion(lookupColumnCmd, "pos", noFileCompletion)

	lookupColumnCmd.RunE = func(cmd *cobra.Command, args []string) error {
		hasPos := cmd.Flags().Changed("pos")
		if hasPos && len(args) > 0 {
			return fmt.Errorf("cannot combine root_id argument with --pos flag")
		}
		if !hasPos && len(args) == 0 {
			return fmt.Errorf("provide a root_id argument or use --pos x,y,z")
		}

		// A changed-but-empty --pos fails as it does for root-info instead of
		// falling through to the root-ID path.
		var x, y, z float64
		if hasPos {
			if strings.TrimSpace(lookupColumnPos) == "" {
				return fmt.Errorf("--pos requires a position as x,y,z")
			}
			px, py, pz, err := parsePos(lookupColumnPos)
			if err != nil {
				return err
			}
			x, y, z = px, py, pz
		}

		// Reuse root-info's data source and nearest-column lookup, so this
		// deprecated command cannot drift from the command that replaced it.
		data, err := rootInfoNewDataSource()
		if err != nil {
			return err
		}
		if !hasPos {
			rootID := args[0]
			row, err := data.QueryNeuronInfo(rootID)
			if err != nil {
				return err
			}
			if row == nil {
				return fmt.Errorf("no neuron found with root_id %q", rootID)
			}
			if !row.HasPosition() {
				return fmt.Errorf("neuron %s has no position coordinates", rootID)
			}
			x, y, z = row.X, row.Y, row.Z
		}

		nearest, nearestErr, err := fetchNearestColumnAt(data, x, y, z, "")
		if err != nil {
			return err
		}
		if nearestErr != "" {
			return errors.New(nearestErr)
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), formatLookupColumnOutput(nearest.Region, nearest.RootID))
		return err
	}

	rootCmd.AddCommand(lookupColumnCmd)
}

// formatLookupColumnOutput keeps the deprecated command's output shape: the
// resolved region and the nearest EPG/PEG root ID, tab-separated.
func formatLookupColumnOutput(region, rootID string) string {
	return fmt.Sprintf("%s\t%s", textout.Sanitize(region), textout.Sanitize(rootID))
}
