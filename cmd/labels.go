package cmd

import (
	"fmt"
	"os"
	"time"

	"crantcli/internal/labelhost"

	"github.com/spf13/cobra"
)

var labelsCmd = &cobra.Command{
	Use:   "labels",
	Short: "Manage cell-type label sources created by commands using --labels",
}

func init() {
	var (
		cleanAll       bool
		cleanOlderThan time.Duration
		cleanHook      string
	)

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Delete label sources (gists or hook-published) tracked by crantcli",
		Long: `Delete cell-type label sources that 'add --labels' or
'state-transfer --labels' created and tracked.

By default, deletes tracked sources older than --older-than. Use --all to delete
every tracked source regardless of age. Hook-published sources are cleaned via
the same --labels-hook command used to create them.

Note: deleting a source removes its labels from any saved/shared state that still
references it.`,
		Args: cobra.NoArgs,
	}
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "Delete every tracked label source regardless of age")
	cleanCmd.Flags().DurationVar(&cleanOlderThan, "older-than", 168*time.Hour, "Delete tracked sources older than this (ignored with --all)")
	cleanCmd.Flags().StringVar(&cleanHook, "labels-hook", "", "Hook command used to clean hook-published sources; defaults to $CRANT_LABELS_HOOK")
	mustRegisterFlagCompletion(cleanCmd, "older-than", noFileCompletion)
	mustRegisterFlagCompletion(cleanCmd, "labels-hook", noFileCompletion)

	cleanCmd.RunE = func(cmd *cobra.Command, args []string) error {
		deleted, kept, err := labelhost.Clean(cleanAll, cleanOlderThan, resolveLabelsHook(cleanHook))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Deleted %d label source(s); %d still tracked\n", deleted, kept)
		return nil
	}

	labelsCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(labelsCmd)
}

func resolveLabelsHook(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("CRANT_LABELS_HOOK")
}
