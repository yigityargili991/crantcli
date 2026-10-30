// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"crantcli/internal/nglstate"

	"github.com/spf13/cobra"
)

// changeDefStateModes are the flags that take the place of the JSON-state
// argument. ask reads the same names from the mutuallyExclusive annotation.
var changeDefStateModes = []string{"builtin", "show", "reset"}

var changeDefStateCmd = &cobra.Command{
	Use:   "change-def-state [json-state]",
	Short: "Set the default Neuroglancer state",
	Long: `Set or update the default Neuroglancer JSON state used when no other state
source is available (no --state, piped JSON, or clipboard URL).

Pass the full JSON state as an argument, or pass a path to a JSON file. Use
--builtin to print the built-in template without reading or writing any state.`,
	Example: `  crantcli change-def-state "$(crantcli change-def-state --builtin)"
  crantcli change-def-state /path/to/state.json
  crantcli change-def-state --show
  crantcli change-def-state --reset
  crantcli change-def-state --builtin`,
	// ask reads this group: Cobra does not export MarkFlagsMutuallyExclusive.
	Annotations: map[string]string{"mutuallyExclusive": strings.Join(changeDefStateModes, ",")},
	Args:        changeDefStateArgs,
	RunE:        runChangeDefState,
}

// errChangeDefStateInput is returned when neither a JSON state nor a mode flag
// was given. changeDefStateArgs normally rejects that before RunE; RunE only
// sees it for an explicit --builtin=false (or --show=false / --reset=false).
var errChangeDefStateInput = errors.New("provide a JSON state, or use --builtin / --show / --reset")

// changeDefStateArgs requires a JSON state argument or one of the mode flags.
// Cobra validates arguments before RunE, and ask probes this validator, so the
// mode flags become known alternatives to the argument: ask's grammar needs one
// of them and never offers the bare command the CLI rejects.
func changeDefStateArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
		return err
	}
	if len(args) > 0 {
		return nil
	}
	for _, name := range changeDefStateModes {
		if cmd.Flags().Changed(name) {
			return nil
		}
	}
	return errChangeDefStateInput
}

var (
	changeDefStateBuiltin bool
	changeDefStateShow    bool
	changeDefStateReset   bool
)

func init() {
	changeDefStateCmd.Flags().BoolVar(&changeDefStateBuiltin, "builtin", false, "Print the built-in default state template")
	changeDefStateCmd.Flags().BoolVar(&changeDefStateShow, "show", false, "Show the current default state")
	changeDefStateCmd.Flags().BoolVar(&changeDefStateReset, "reset", false, "Reset to built-in default")
	changeDefStateCmd.MarkFlagsMutuallyExclusive(changeDefStateModes...)
	rootCmd.AddCommand(changeDefStateCmd)
}

func runChangeDefState(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	if changeDefStateBuiltin {
		if len(args) > 0 {
			return fmt.Errorf("cannot combine --builtin with a JSON or file argument")
		}
		_, err := fmt.Fprint(out, string(nglstate.DefaultScene))
		return err
	}

	if (changeDefStateShow || changeDefStateReset) && len(args) > 0 {
		return fmt.Errorf("cannot combine --show or --reset with a JSON argument")
	}

	if changeDefStateReset {
		if err := nglstate.WriteDefaultState(nil); err != nil {
			return err
		}
		fmt.Fprintln(out, "Reset to built-in default state")
		return nil
	}

	if changeDefStateShow {
		data, err := nglstate.ReadDefaultState()
		if err != nil {
			return err
		}
		if len(data) == 0 {
			fmt.Fprintln(out, "No custom default state set (using built-in template)")
		} else {
			fmt.Fprint(out, string(data))
		}
		return nil
	}

	// changeDefStateArgs rejects a bare run; this guard only catches an explicit
	// --builtin=false (or --show=false / --reset=false), which Args counts as a
	// set mode flag while the parsed value stays false.
	if len(args) == 0 {
		return errChangeDefStateInput
	}

	raw := strings.TrimSpace(args[0])

	// If the argument is not JSON, treat it as a file path
	if !strings.HasPrefix(raw, "{") {
		data, err := os.ReadFile(raw)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", raw, err)
		}
		raw = strings.TrimSpace(string(data))
	}

	// Validate it's valid JSON
	var state map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if state == nil {
		return fmt.Errorf("invalid JSON: expected a JSON object")
	}

	// Pretty-print for storage
	pretty, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := nglstate.WriteDefaultState(append(pretty, '\n')); err != nil {
		return err
	}

	fmt.Fprintln(out, "Default state updated")
	return nil
}
