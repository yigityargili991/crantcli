package cmd

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

var completionFields = []string{
	"super_class",
	"cell_class",
	"cell_type",
	"cell_subtype",
	"cell_instance",
	"side",
	"region",
	"tract",
	"nerve",
	"hemilineage",
	"proofread",
}

// completeColorByFields completes --color-by, including the field after a
// comma ("cell_type,cell_" -> "cell_type,cell_subtype"). Fields already named
// in the value are dropped, since --color-by rejects repeats.
func completeColorByFields(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	prefix := ""
	if comma := strings.LastIndex(toComplete, ","); comma >= 0 {
		prefix = toComplete[:comma+1]
	}
	if strings.Count(prefix, ",") >= maxAddColorByFields {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	used := strings.Split(prefix, ",")
	// A continuous field varies per neuron rather than forming groups, so it
	// never pairs with a second field.
	for _, field := range used {
		if continuousAddColorByFields[field] {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}

	values := make([]string, 0, len(addColorByFields))
	for _, field := range addColorByFields {
		if slices.Contains(used, field) {
			continue
		}
		// After a comma only fields that can be the tone remain: a continuous
		// field stands alone, and the query group is always the outer level.
		if prefix != "" && (continuousAddColorByFields[field] || field == queryGroupColorByField) {
			continue
		}
		values = append(values, prefix+field)
	}
	return filterCompletionValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

var colorCompletions = []string{
	"colored",
	"blue",
	"red",
	"green",
	"turquoise",
	"orange",
	"purple",
	"yellow",
	"pink",
	"brown",
	"indigo",
	"teal",
	"lime",
}

var completionClient = seatable.NewClient
var completionSelectOptions = selectOptionsForCompletion
var completionQueryDistinct = seatable.QueryDistinct

func completeListFields(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletionValues(completionFields, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func noFileCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeStaticValues(values []string) cobra.CompletionFunc {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return filterCompletionValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func completeDistinctField(field string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		values, directive := completeDistinctValues(field, completionFilters(cmd, field), toComplete)
		return values, directive
	}
}

func completeDistinctValues(field string, filters *seatable.Filters, toComplete string) ([]string, cobra.ShellCompDirective) {
	client, err := completionClient()
	if err != nil {
		return unavailableCompletionHelp(), cobra.ShellCompDirectiveNoFileComp
	}

	if filters == nil || !filters.HasAny() {
		values, err := completionSelectOptions(client, field)
		if err != nil {
			return unavailableCompletionHelp(), cobra.ShellCompDirectiveNoFileComp
		}
		if len(values) > 0 {
			return filterCompletionValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
		}
	}

	resp, err := completionQueryDistinct(client, field, filters, false)
	if err != nil {
		return unavailableCompletionHelp(), cobra.ShellCompDirectiveNoFileComp
	}

	seen := make(map[string]bool, len(resp.Results))
	values := make([]string, 0, len(resp.Results))
	for _, row := range resp.Results {
		value := strings.TrimSpace(fmt.Sprint(row[field]))
		if value == "" || value == "<nil>" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	sort.Strings(values)

	return filterCompletionValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func selectOptionsForCompletion(client *seatable.Client, field string) ([]string, error) {
	meta, err := client.FetchMetadata()
	if err != nil {
		return nil, err
	}

	optionMap := seatable.SelectOptionMap(meta, field)
	values := make([]string, 0, len(optionMap))
	for _, value := range optionMap {
		if strings.TrimSpace(value) == "" {
			continue
		}
		values = append(values, value)
	}
	sort.Strings(values)
	return values, nil
}

func unavailableCompletionHelp() []string {
	return cobra.AppendActiveHelp(nil, "data completions unavailable; run 'crantcli setup' or check your network")
}

func completionFilters(cmd *cobra.Command, omitField string) *seatable.Filters {
	filters := &seatable.Filters{}

	if omitField != "super_class" {
		filters.SuperClass = stringFlagValue(cmd, "super-class")
	}
	if omitField != "cell_class" {
		filters.CellClass = singleStringFlagValue(cmd, "cell-class")
	}
	if omitField != "cell_type" {
		filters.CellType = singleStringFlagValue(cmd, "cell-type")
	}
	if omitField != "cell_subtype" {
		filters.CellSubtype = singleStringFlagValue(cmd, "cell-subtype")
	}
	if omitField != "side" {
		filters.Side = stringFlagValue(cmd, "side")
	}
	if omitField != "region" {
		filters.Region = singleStringFlagValue(cmd, "region")
		if filters.Region == "" {
			filters.Region = singleStringFlagValue(cmd, "bundle")
		}
	}
	if omitField != "tract" {
		filters.Tract = stringFlagValue(cmd, "tract")
	}
	if omitField != "nerve" {
		filters.Nerve = stringFlagValue(cmd, "nerve")
	}
	if omitField != "hemilineage" {
		filters.Hemilineage = stringFlagValue(cmd, "hemilineage")
	}
	if omitField != "proofread" {
		filters.Proofread = stringFlagValue(cmd, "proofread")
	}

	return filters
}

func stringFlagValue(cmd *cobra.Command, name string) string {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return ""
	}
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return value
}

func singleStringFlagValue(cmd *cobra.Command, name string) string {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return ""
	}

	if values, err := cmd.Flags().GetStringArray(name); err == nil {
		if len(values) == 1 {
			return values[0]
		}
		return ""
	}

	return stringFlagValue(cmd, name)
}

func filterCompletionValues(values []string, toComplete string) []string {
	if toComplete == "" {
		return values
	}

	prefix := strings.ToLower(toComplete)
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		matchValue := value
		if tab := strings.IndexByte(matchValue, '\t'); tab >= 0 {
			matchValue = matchValue[:tab]
		}
		if strings.HasPrefix(strings.ToLower(matchValue), prefix) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func mustRegisterFlagCompletion(cmd *cobra.Command, flagName string, fn cobra.CompletionFunc) {
	if err := cmd.RegisterFlagCompletionFunc(flagName, fn); err != nil {
		panic(fmt.Sprintf("registering completion for %s --%s: %v", cmd.Name(), flagName, err))
	}
}

func registerClassificationFlagCompletions(cmd *cobra.Command, flagNames ...string) {
	for _, flagName := range flagNames {
		field, ok := completionFieldForFlag(flagName)
		if !ok {
			continue
		}
		mustRegisterFlagCompletion(cmd, flagName, completeDistinctField(field))
	}
}

func completionFieldForFlag(flagName string) (string, bool) {
	switch flagName {
	case "super-class":
		return "super_class", true
	case "cell-class":
		return "cell_class", true
	case "cell-type":
		return "cell_type", true
	case "cell-subtype":
		return "cell_subtype", true
	case "side":
		return "side", true
	case "region", "bundle":
		return "region", true
	case "tract":
		return "tract", true
	case "nerve":
		return "nerve", true
	case "hemilineage":
		return "hemilineage", true
	case "proofread":
		return "proofread", true
	default:
		return "", false
	}
}
