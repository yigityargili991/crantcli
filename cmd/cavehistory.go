package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"crantcli/internal/cave"
	"crantcli/internal/nglstate"
	"crantcli/internal/textout"

	"github.com/spf13/cobra"
)

type caveHistoryClient interface {
	GetRootChangeLog(rootID uint64, filtered bool) ([]cave.ChangeLogRow, error)
}

type caveHistoryOptions struct {
	JSON         bool
	Filtered     bool
	Open         bool
	Output       string
	HistoryLimit int
	Compact      bool
	Labels       bool
	LabelsTTL    time.Duration
	LabelsHook   string
}

var caveHistoryDeliverState = nglstate.DeliverState
var caveHistoryAttachLabels = attachSegmentPropertyLabels

type caveHistoryResult struct {
	RootID  string             `json:"root_id"`
	Entries []caveHistoryEntry `json:"entries"`
}

var caveHistoryCmd = &cobra.Command{
	Use:   "cave-history [root_id...]",
	Short: "Show CAVE edit history for root IDs",
	Long: `Show CAVE tabular changelog rows for one or more root IDs.

By default, only edits that affect the final state of the queried root are
included. Use --unfiltered to include broader split/merge history for objects
that were once associated with the queried root.

Use --open or --output to create a fresh Neuroglancer history state. By default,
one compact 3D history layer lists the historical root IDs. Toggle roots in its
Seg. panel to compare stages; only the latest before roots start visible.
Use --compact=false for separate before/after layers with 2D segmentation.
After roots are those reported by CAVE, which may omit split-off objects outside
the queried lineage.

With --open and no --output, the state URL is also copied to the clipboard.
Add --labels to publish editor/time labels using the same hosting as add --labels.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			return err
		}
		unfiltered, err := cmd.Flags().GetBool("unfiltered")
		if err != nil {
			return err
		}
		opts := caveHistoryOptions{JSON: asJSON, Filtered: !unfiltered}
		opts.Open, _ = cmd.Flags().GetBool("open")
		opts.Output, _ = cmd.Flags().GetString("output")
		opts.HistoryLimit, _ = cmd.Flags().GetInt("history-limit")
		opts.Compact, _ = cmd.Flags().GetBool("compact")
		opts.Labels, _ = cmd.Flags().GetBool("labels")
		opts.LabelsTTL, _ = cmd.Flags().GetDuration("labels-ttl")
		opts.LabelsHook, _ = cmd.Flags().GetString("labels-hook")
		if err := opts.validate(); err != nil {
			return err
		}

		caveClient, err := cave.NewClient()
		if err != nil {
			return err
		}

		return runCaveHistory(cmd.OutOrStdout(), cmd.ErrOrStderr(), caveClient, args, opts)
	},
}

func init() {
	caveHistoryCmd.Flags().Bool("json", false, "Print JSON output")
	caveHistoryCmd.Flags().Bool("unfiltered", false, "Include unfiltered split/merge history")
	caveHistoryCmd.Flags().Bool("open", false, "Open a colored history state in the default browser")
	caveHistoryCmd.Flags().StringP("output", "o", "", "Save the history state as Neuroglancer JSON")
	caveHistoryCmd.Flags().Int("history-limit", 10, "Latest edits per root in the viewer state (0 for all; does not limit table/JSON)")
	caveHistoryCmd.Flags().Bool("compact", true, "Use one 3D history layer; set false for separate stages with 2D segmentation (with --open or --output)")
	caveHistoryCmd.Flags().Bool("labels", false, "Publish root labels with edit type, editor, and UTC time (requires --open or --output)")
	caveHistoryCmd.Flags().Duration("labels-ttl", 168*time.Hour, "Delete previously-created label sources older than this on each --labels run")
	caveHistoryCmd.Flags().String("labels-hook", "", "Command to publish/clean labels instead of a GitHub gist; defaults to $CRANT_LABELS_HOOK")
	for _, flag := range []string{"history-limit", "labels-ttl", "labels-hook"} {
		mustRegisterFlagCompletion(caveHistoryCmd, flag, noFileCompletion)
	}
	caveHistoryCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(caveHistoryCmd)
}

func runCaveHistory(out, errOut io.Writer, client caveHistoryClient, args []string, opts caveHistoryOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}
	results, err := fetchCaveHistory(client, args, opts.Filtered)
	if err != nil {
		return err
	}
	if opts.Open || opts.Output != "" {
		return deliverCaveHistoryState(errOut, results, opts)
	}

	if opts.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	return writeCaveHistoryTable(out, errOut, results)
}

func (opts caveHistoryOptions) validate() error {
	view := opts.Open || opts.Output != ""
	if opts.JSON && view {
		return fmt.Errorf("--json cannot be combined with --open or --output")
	}
	if opts.Labels && !view {
		return fmt.Errorf("--labels requires --open or --output")
	}
	if opts.HistoryLimit < 0 {
		return fmt.Errorf("--history-limit must be non-negative")
	}
	if opts.Labels && opts.LabelsTTL < 0 {
		return fmt.Errorf("--labels-ttl must be non-negative")
	}
	return nil
}

func fetchCaveHistory(client caveHistoryClient, args []string, filtered bool) ([]caveHistoryResult, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("provide at least one root_id")
	}

	results := make([]caveHistoryResult, len(args))
	for i, arg := range args {
		rootID, err := parseCaveHistoryRootID(arg)
		if err != nil {
			return nil, err
		}

		rows, err := client.GetRootChangeLog(rootID, filtered)
		if err != nil {
			return nil, fmt.Errorf("fetching history for root_id %s: %w", arg, err)
		}

		results[i] = caveHistoryResult{
			RootID:  strconv.FormatUint(rootID, 10),
			Entries: caveRowsToHistoryEntries(rows),
		}
	}
	return results, nil
}

func parseCaveHistoryRootID(raw string) (uint64, error) {
	rootID, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid root_id %q: %w", raw, err)
	}
	return rootID, nil
}

func writeCaveHistoryTable(out, errOut io.Writer, results []caveHistoryResult) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	headerPrinted := false
	rowCount := 0

	for _, result := range results {
		for _, entry := range result.Entries {
			if !headerPrinted {
				fmt.Fprintln(w, "root_id\toperation_id\ttimestamp_utc\ttype\tbefore_root_ids\tafter_root_ids\tuser_id\tuser_name\tuser_affiliation")
				headerPrinted = true
			}
			rowCount++
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
				result.RootID,
				entry.OperationID,
				entry.TimestampUTC,
				entry.Type,
				formatHistoryRootList(entry.BeforeRootIDs),
				formatHistoryRootList(entry.AfterRootIDs),
				entry.UserID,
				textout.Sanitize(entry.UserName),
				textout.Sanitize(entry.UserAffiliation),
			)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	if rowCount == 0 {
		fmt.Fprintln(errOut, "no history found")
	}
	return nil
}
