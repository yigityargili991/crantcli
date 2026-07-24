package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"crantcli/internal/clipboard"
	"crantcli/internal/config"
	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

var stateTransferCmd = &cobra.Command{
	Use:   "state-transfer",
	Short: "Build a Neuroglancer state from IDs in the clipboard",
	Long: `Read root IDs from the clipboard, inject them into a Neuroglancer state,
and copy the resulting state URL back to the clipboard.

The clipboard should contain root IDs separated by whitespace, newlines,
or commas. The command loads a base state (from --state, piped stdin,
user-configured default, or built-in template), injects the IDs into the
segmentation layer, and writes the resulting Neuroglancer URL to the clipboard.`,
	Example: `  # Copy some root IDs, then:
  crantcli state-transfer

  # Use a specific base state
  crantcli state-transfer -s base.json

  # Target a specific segmentation layer
  crantcli state-transfer -l "merge-biased seg"

  # Attach cell-type labels to the clipboard root IDs
  crantcli state-transfer --labels

  # Write to file instead of clipboard
  crantcli state-transfer -o output.json`,
}

func init() {
	var (
		stState      string
		stOutput     string
		stLayer      string
		stColor      string
		stLabels     bool
		stLabelsTTL  time.Duration
		stLabelsHook string
	)

	stateTransferCmd.Flags().StringVarP(&stState, "state", "s", "", "Base Neuroglancer state (URL or file path; default: template)")
	stateTransferCmd.Flags().StringVarP(&stOutput, "output", "o", "", "Output file path (default: clipboard)")
	stateTransferCmd.Flags().StringVarP(&stLayer, "layer", "l", "", "Target segmentation layer name")
	stateTransferCmd.Flags().StringVar(&stColor, "color", "", "Segment color: named color, 'colored' for random, or hex (#ff0000)")
	stateTransferCmd.Flags().BoolVar(&stLabels, "labels", false, "Attach cell-type labels (via an ephemeral secret GitHub gist) so types show next to root IDs in the Seg. panel; requires the gh CLI")
	stateTransferCmd.Flags().DurationVar(&stLabelsTTL, "labels-ttl", 168*time.Hour, "Delete previously-created label sources older than this on each --labels run")
	stateTransferCmd.Flags().StringVar(&stLabelsHook, "labels-hook", "", "Command to publish/clean label sources instead of a GitHub gist (receives info JSON on stdin, prints {\"url\",\"id\"}); defaults to $CRANT_LABELS_HOOK")
	stateTransferCmd.ValidArgsFunction = noFileCompletion
	mustRegisterFlagCompletion(stateTransferCmd, "color", completeStaticValues(colorCompletions))
	mustRegisterFlagCompletion(stateTransferCmd, "layer", noFileCompletion)
	mustRegisterFlagCompletion(stateTransferCmd, "labels-ttl", noFileCompletion)
	mustRegisterFlagCompletion(stateTransferCmd, "labels-hook", noFileCompletion)

	stateTransferCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if !stLabels || config.GetAPIToken() != "" {
			return nil
		}
		return config.RunSetupPrompt()
	}

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

		if stLabels {
			client, err := seatable.NewClient()
			if err != nil {
				return err
			}
			rows, err := seatable.QueryNeuronsByRootIDs(client, ids)
			if err != nil {
				return fmt.Errorf("querying labels for clipboard root IDs: %w", err)
			}
			if len(rows) == 0 {
				return fmt.Errorf("no CRANT metadata found for clipboard root IDs")
			}
			if len(rows) < len(ids) {
				fmt.Fprintf(os.Stderr, "Warning: found CRANT metadata for %d of %d clipboard root IDs\n", len(rows), len(ids))
			}
			if err := attachCellTypeLabels(layer, rows, stLabelsTTL, resolveLabelsHook(stLabelsHook)); err != nil {
				return err
			}
		}

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
