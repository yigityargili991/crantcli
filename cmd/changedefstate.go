package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"crantcli/internal/nglstate"

	"github.com/spf13/cobra"
)

var changeDefStateCmd = &cobra.Command{
	Use:   "change-def-state <json-state>",
	Short: "Set the default Neuroglancer state",
	Long: `Set or update the default Neuroglancer JSON state used when no other state
source is available (no --state, no clipboard, no session cache).

Pass the full JSON state as an argument, or pass a path to a JSON file.

Examples:
  crantcli change-def-state '{"dimensions":...}'
  crantcli change-def-state /path/to/state.json
  crantcli change-def-state --show
  crantcli change-def-state --reset`,
	Args: cobra.MaximumNArgs(1),
	RunE: runChangeDefState,
}

var (
	changeDefStateShow  bool
	changeDefStateReset bool
)

func init() {
	changeDefStateCmd.Flags().BoolVar(&changeDefStateShow, "show", false, "Show the current default state")
	changeDefStateCmd.Flags().BoolVar(&changeDefStateReset, "reset", false, "Reset to built-in default")
	changeDefStateCmd.MarkFlagsMutuallyExclusive("show", "reset")
	rootCmd.AddCommand(changeDefStateCmd)
}

func runChangeDefState(cmd *cobra.Command, args []string) error {
	if (changeDefStateShow || changeDefStateReset) && len(args) > 0 {
		return fmt.Errorf("cannot combine --show or --reset with a JSON argument")
	}

	if changeDefStateReset {
		if err := nglstate.WriteDefaultState(nil); err != nil {
			return err
		}
		fmt.Println("Reset to built-in default state")
		return nil
	}

	if changeDefStateShow {
		data, err := nglstate.ReadDefaultState()
		if err != nil {
			return err
		}
		if len(data) == 0 {
			fmt.Println("No custom default state set (using built-in template)")
		} else {
			fmt.Print(string(data))
		}
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a JSON state, or use --show / --reset")
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

	// Pretty-print for storage
	pretty, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := nglstate.WriteDefaultState(append(pretty, '\n')); err != nil {
		return err
	}

	fmt.Println("Default state updated")
	return nil
}
