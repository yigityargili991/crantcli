package cmd

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"crantcli/internal/labelhost"
	"crantcli/internal/segprops"

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

// noLabelTags is the --label-tags value that publishes labels without any
// filterable tag chips.
const noLabelTags = "none"

// labelFieldList renders the fields --label-by and --label-tags accept.
var labelFieldList = strings.Join(segprops.Fields, ", ")

// defaultLabelBy and defaultLabelTags render the built-in label configuration as
// flag values, so --help states what an omitted flag does and every run parses
// the same shape of input.
var (
	defaultLabelBy   = strings.Join(defaultLabelFields(), ",")
	defaultLabelTags = strings.Join(segprops.DefaultOptions().TagFields, ",")
)

// defaultLabelFields is the built-in label field followed by its fallbacks, in
// the order --label-by takes them.
func defaultLabelFields() []string {
	opts := segprops.DefaultOptions()
	return append([]string{opts.LabelField}, opts.LabelFallbacks...)
}

// registerLabelFlags declares the flags shared by every command that publishes
// labels, keeping their names, defaults, and help text identical.
func registerLabelFlags(cmd *cobra.Command, enabled *bool, ttl *time.Duration, hook, by, tags *string) {
	cmd.Flags().BoolVar(enabled, "labels", false, "Attach cell-type labels (via an ephemeral secret GitHub gist) so types show next to root IDs in the Seg. panel; requires the gh CLI, or a publish hook via --labels-hook/$CRANT_LABELS_HOOK")
	cmd.Flags().DurationVar(ttl, "labels-ttl", 168*time.Hour, "Delete previously-created label sources older than this on each --labels run")
	cmd.Flags().StringVar(hook, "labels-hook", "", "Command to publish/clean label sources instead of a GitHub gist (receives info JSON on stdin, prints {\"url\",\"id\"}); defaults to $CRANT_LABELS_HOOK")
	cmd.Flags().StringVar(by, "label-by", defaultLabelBy, "Field shown as the --labels label: "+labelFieldList+". Further comma-separated fields are fallbacks, each tried when the previous one is empty")
	cmd.Flags().StringVar(tags, "label-tags", defaultLabelTags, "Comma-separated fields published as filterable tag chips ("+labelFieldList+"), or '"+noLabelTags+"' for no tags")

	mustRegisterFlagCompletion(cmd, "labels-ttl", noFileCompletion)
	mustRegisterFlagCompletion(cmd, "labels-hook", noFileCompletion)
	mustRegisterFlagCompletion(cmd, "label-by", completeLabelFields(nil))
	mustRegisterFlagCompletion(cmd, "label-tags", completeLabelFields([]string{noLabelTags}))
}

// resolveLabelOptions turns --label-by and --label-tags into the segment
// properties configuration. --label-by names the label field first and its
// fallbacks after it; --label-tags names the filterable tag fields, or
// "none"/empty for a label-only source.
func resolveLabelOptions(labelBy, labelTags string) (segprops.Options, error) {
	labelFields, err := parseLabelFields("label-by", labelBy)
	if err != nil {
		return segprops.Options{}, err
	}
	if len(labelFields) == 0 {
		return segprops.Options{}, fmt.Errorf("invalid --label-by %q: name at least one field; valid fields: %s", labelBy, labelFieldList)
	}

	opts := segprops.Options{LabelField: labelFields[0], LabelFallbacks: labelFields[1:]}
	if strings.TrimSpace(labelTags) == noLabelTags {
		return opts, nil
	}
	tagFields, err := parseLabelFields("label-tags", labelTags)
	if err != nil {
		return segprops.Options{}, err
	}
	opts.TagFields = tagFields
	return opts, nil
}

// parseLabelFields splits a comma-separated field list, rejecting unknown and
// repeated fields. A repeat would name the same value twice, which reads as a
// mistake rather than a request.
func parseLabelFields(flagName, value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parts := strings.Split(value, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			return nil, fmt.Errorf("invalid --%s %q: empty field", flagName, value)
		}
		if !segprops.ValidField(field) {
			return nil, fmt.Errorf("invalid --%s %q; valid fields: %s", flagName, field, labelFieldList)
		}
		if slices.Contains(fields, field) {
			return nil, fmt.Errorf("invalid --%s %q: %q is repeated", flagName, value, field)
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// warnUnshapedLabelFlags names label-shaping flags set on a run that publishes
// nothing, since they would otherwise be dropped without a word.
func warnUnshapedLabelFlags(w io.Writer, cmd *cobra.Command, labels bool) {
	if labels || cmd == nil {
		return
	}

	var set []string
	for _, name := range []string{"label-by", "label-tags"} {
		if cmd.Flags().Changed(name) {
			set = append(set, "--"+name)
		}
	}
	if len(set) == 0 {
		return
	}

	verb := "have"
	if len(set) == 1 {
		verb = "has"
	}
	fmt.Fprintf(w, "Warning: %s %s no effect without --labels\n", strings.Join(set, " and "), verb)
}

// labelFieldName names the field a published source labels by, mirroring the
// default segprops applies when the field is unset.
func labelFieldName(opts segprops.Options) string {
	if opts.LabelField == "" {
		return segprops.DefaultOptions().LabelField
	}
	return opts.LabelField
}
