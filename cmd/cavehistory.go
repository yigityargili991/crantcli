package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"crantcli/internal/cave"
	"crantcli/internal/textout"

	"github.com/spf13/cobra"
)

type caveHistoryClient interface {
	GetRootChangeLog(rootID uint64, filtered bool) ([]cave.ChangeLogRow, error)
}

type caveHistoryOptions struct {
	JSON     bool
	Filtered bool
}

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
that were once associated with the queried root.`,
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

		caveClient, err := cave.NewClient()
		if err != nil {
			return err
		}

		return runCaveHistory(os.Stdout, os.Stderr, caveClient, args, caveHistoryOptions{
			JSON:     asJSON,
			Filtered: !unfiltered,
		})
	},
}

func init() {
	caveHistoryCmd.Flags().Bool("json", false, "Print JSON output")
	caveHistoryCmd.Flags().Bool("unfiltered", false, "Include unfiltered split/merge history")
	caveHistoryCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(caveHistoryCmd)
}

func runCaveHistory(out, errOut io.Writer, client caveHistoryClient, args []string, opts caveHistoryOptions) error {
	results, err := fetchCaveHistory(client, args, opts.Filtered)
	if err != nil {
		return err
	}

	if opts.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	return writeCaveHistoryTable(out, errOut, results)
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
