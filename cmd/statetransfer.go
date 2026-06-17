package cmd

import (
	"fmt"
	"os"
	"strings"

	"crantcli/internal/clipboard"
	"crantcli/internal/nglstate"

	"github.com/spf13/cobra"
)

var stateTransferCmd = &cobra.Command{
	Use:   "state-transfer",
	Short: "Build a Neuroglancer state from IDs in the clipboard",
	Long: `Read root IDs from the clipboard, inject them into a Neuroglancer state,
and copy the resulting state URL back to the clipboard.

The clipboard should contain root IDs separated by whitespace, newlines,
or commas. The command loads a base state (from --state, default template,
or user-configured default), injects the IDs into the segmentation layer,
and writes the resulting Neuroglancer URL to the clipboard.

Examples:
  # Copy some root IDs, then:
  crantcli state-transfer

  # Use a specific base state
  crantcli state-transfer -s base.json

  # Target a specific segmentation layer
  crantcli state-transfer -l "my layer"

  # Write to file instead of clipboard
  crantcli state-transfer -o output.json`,
}

func init() {
	var (
		stState  string
		stOutput string
		stLayer  string
		stColor  string
	)

	stateTransferCmd.Flags().StringVarP(&stState, "state", "s", "", "Base Neuroglancer state (URL or file path; default: template)")
	stateTransferCmd.Flags().StringVarP(&stOutput, "output", "o", "", "Output file path (default: clipboard)")
	stateTransferCmd.Flags().StringVarP(&stLayer, "layer", "l", "", "Target segmentation layer name")
	stateTransferCmd.Flags().StringVar(&stColor, "color", "", "Segment color: named color, 'colored' for random, or hex (#ff0000)")
	stateTransferCmd.ValidArgsFunction = noFileCompletion
	mustRegisterFlagCompletion(stateTransferCmd, "color", completeStaticValues(colorCompletions))
	mustRegisterFlagCompletion(stateTransferCmd, "layer", noFileCompletion)

	stateTransferCmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Read IDs from clipboard
		clip, err := clipboard.Read()
		if err != nil {
			return fmt.Errorf("reading clipboard: %w", err)
		}
		if strings.TrimSpace(clip) == "" {
			return fmt.Errorf("clipboard is empty — copy some root IDs first")
		}

		ids := parseIDs(clip)
		if len(ids) == 0 {
			return fmt.Errorf("no valid IDs found in clipboard")
		}

		fmt.Fprintf(os.Stderr, "Read %d IDs from clipboard\n", len(ids))

		normalizedColor, err := nglstate.NormalizeColorInput(stColor)
		if err != nil {
			return err
		}

		// Load base state (always use generate=true to prefer template over clipboard,
		// since the clipboard currently holds IDs, not a Neuroglancer URL)
		result, err := nglstate.LoadState(stState, true)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "State loaded from %s\n", result.Source)

		// Find segmentation layer and inject IDs
		layer, _, err := nglstate.FindSegmentationLayer(result.State, stLayer)
		if err != nil {
			return err
		}

		nglstate.AddSegments(layer, ids, true)
		nglstate.SetSegmentColor(layer, ids, normalizedColor)

		// Write output
		// Force clipboard/URL output when no --output flag is given,
		// regardless of how the state was loaded.
		if stOutput == "" {
			result.Source = nglstate.SourceTemplate
		}
		if err := nglstate.WriteState(result, stOutput); err != nil {
			return err
		}

		return nil
	}

	rootCmd.AddCommand(stateTransferCmd)
}

// parseIDs splits clipboard text into individual root IDs.
// Accepts whitespace, newlines, commas, or any mix as separators.
func parseIDs(text string) []string {
	// Replace commas with spaces so we can split uniformly
	text = strings.ReplaceAll(text, ",", " ")
	parts := strings.Fields(text)

	seen := make(map[string]bool, len(parts))
	var ids []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		ids = append(ids, p)
	}
	return ids
}
