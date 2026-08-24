package cmd

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"crantcli/internal/clipboard"
	"crantcli/internal/labelhost"
	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"
	"crantcli/internal/segprops"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Query neurons and inject root IDs into a Neuroglancer state",
	Long: `Query the CRANT dataset for neurons matching the given filters and inject
their root IDs into a Neuroglancer state.

Smart input resolution (when no --state is given):

  1. Check stdin for piped JSON
  2. Check clipboard for a Neuroglancer URL (read implicitly)
  3. Fall back to the configured or built-in CRANT scene template

With no --output, the resulting Neuroglancer URL is written back to the
clipboard, overwriting its current contents.`,
	Example: `  # Smart: checks clipboard for a Neuroglancer URL, injects, and copies back
  crantcli add --cell-type ER

  # Explicit file I/O
  crantcli add --cell-type ER -s state.json -o modified.json

  # Generate a fresh state
  crantcli add --cell-type ER --generate

  # Open the updated state in a browser
  crantcli add --cell-type ER --open

  # Print root IDs without manipulating a state
  crantcli add --cell-type ER --root-ids-only

  # Add multiple cell types with per-group coloring
  crantcli add --cell-type ER --cell-type EPG/PEG --color colored

  # Add sensory neurons and color by cell_type
  crantcli add --super-class sensory --color-by cell_type

  # Show cell types next to root IDs in the Seg. panel (requires the gh CLI)
  crantcli add --cell-type ER --labels

  # Add all neurons annotated to bundle/region LX
  crantcli add --bundle LX

  # Stack classifiers as a union
  crantcli add --cell-class LNO --cell-subtype PFNc --cell-subtype PFNm3 --cell-type PEN --color-by cell_subtype

  # Intersect classifiers instead
  crantcli add --intersect --cell-class ER --cell-type ER

  # Color by cell_type, then vary tone by cell_subtype within each type
  crantcli add --super-class sensory --color-by cell_type,cell_subtype

  # Give each query group a hue and each subtype inside it a tone
  crantcli add --cell-class LNO --cell-type PEN --color-by group,cell_subtype`,
	Annotations: map[string]string{"requiresToken": "true"},
}

var (
	addSuperClass   string
	addCellClasses  []string
	addCellTypes    []string
	addCellSubtypes []string
	addIntersect    bool
	addSide         string
	addRegions      []string
	addBundles      []string
	addTract        string
	addProofread    string
	addState        string
	addGenerate     bool
	addOutput       string
	addLayer        string
	addColor        string
	addColorBy      string
	addReplace      bool
	addRootIDsOnly  bool
	addOpen         bool
	addColorSub     bool
	addLabels       bool
	addLabelsTTL    time.Duration
	addLabelsHook   string

	addNewClient             = seatable.NewClient
	addQueryNeurons          = seatable.QueryNeurons
	addLoadState             = nglstate.LoadState
	addFindSegmentationLayer = nglstate.FindSegmentationLayer
	addDeliverState          = nglstate.DeliverState
	addClipboardWrite        = clipboard.WriteText
)

func init() {
	addCmd.Flags().StringVar(&addSuperClass, "super-class", "", "Filter by super_class")
	addCmd.Flags().StringArrayVar(&addCellClasses, "cell-class", nil, "Filter by cell_class (repeatable for multiple classes)")
	addCmd.Flags().StringArrayVar(&addCellTypes, "cell-type", nil, "Filter by cell_type (repeatable for multiple types)")
	addCmd.Flags().StringArrayVar(&addCellSubtypes, "cell-subtype", nil, "Filter by cell_subtype (repeatable for multiple subtypes)")
	addCmd.Flags().BoolVar(&addIntersect, "intersect", false, "Intersect --cell-class/--cell-type/--cell-subtype as a cross-product (AND) instead of the default union (OR, each value its own group); rarely needed since these classifiers are hierarchical. Other filters (--super-class, --side, --region, ...) always apply to every group")
	addCmd.Flags().StringVar(&addSide, "side", "", "Filter by side")
	addCmd.Flags().StringArrayVar(&addRegions, "region", nil, "Filter by region (repeatable for multiple regions)")
	addCmd.Flags().StringArrayVar(&addBundles, "bundle", nil, "Filter by bundle region annotation (repeatable alias of --region, e.g. LX)")
	addCmd.Flags().StringVar(&addTract, "tract", "", "Filter by tract")
	addCmd.Flags().StringVar(&addProofread, "proofread", "", "Filter by proofread status")
	addCmd.Flags().StringVarP(&addState, "state", "s", "", "Neuroglancer state (URL or file path)")
	addCmd.Flags().BoolVarP(&addGenerate, "generate", "g", false, "Use the configured or built-in default state instead of the clipboard")
	addCmd.Flags().StringVarP(&addOutput, "output", "o", "", "Output file path (default: clipboard or stdout)")
	addCmd.Flags().StringVarP(&addLayer, "layer", "l", "", "Target segmentation layer name")
	addCmd.Flags().StringVar(&addColor, "color", "", "Segment color: named (blue, red, green, turquoise, orange, purple, yellow, pink, brown, indigo, teal, lime) with auto-toning, 'colored' for per-group palette cycling, or hex (#ff0000)")
	addCmd.Flags().StringVar(&addColorBy, "color-by", "", "Color matched rows by field: "+addColorByFieldList+". Two comma-separated fields nest, the first choosing the hue and the second the tone within it (needs --color colored)")
	addCmd.Flags().BoolVar(&addColorSub, "color-sub", false, "Sub-color neurons by cell_subtype within each query group")
	mustMarkFlagDeprecated(addCmd, "color-sub",
		"use --color-by group,cell_subtype with --color colored, or --color-by cell_subtype with a named family")
	addCmd.Flags().BoolVar(&addReplace, "replace", false, "Replace existing segments instead of appending")
	addCmd.Flags().BoolVar(&addRootIDsOnly, "root-ids-only", false, "Print root IDs and copy them to the clipboard; no state manipulation")
	addCmd.Flags().BoolVar(&addOpen, "open", false, "Open updated Neuroglancer URL in default browser")
	addCmd.Flags().BoolVar(&addLabels, "labels", false, "Attach cell-type labels (via an ephemeral secret GitHub gist) so types show next to root IDs in the Seg. panel; requires the gh CLI, or a publish hook via --labels-hook/$CRANT_LABELS_HOOK")
	addCmd.Flags().DurationVar(&addLabelsTTL, "labels-ttl", 168*time.Hour, "Delete previously-created label sources older than this on each --labels run")
	addCmd.Flags().StringVar(&addLabelsHook, "labels-hook", "", "Command to publish/clean label sources instead of a GitHub gist (receives info JSON on stdin, prints {\"url\",\"id\"}); defaults to $CRANT_LABELS_HOOK")
	addCmd.Args = func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return err
		}
		return validateAddOptions(addRegions, addBundles, addColor, addColorBy, addColorSub)
	}
	addCmd.ValidArgsFunction = noFileCompletion
	registerClassificationFlagCompletions(addCmd,
		"super-class",
		"cell-class",
		"cell-type",
		"cell-subtype",
		"side",
		"region",
		"bundle",
		"tract",
		"proofread",
	)
	mustRegisterFlagCompletion(addCmd, "color", completeStaticValues(colorCompletions))
	mustRegisterFlagCompletion(addCmd, "color-by", completeColorByFields)
	mustRegisterFlagCompletion(addCmd, "layer", noFileCompletion)
	mustRegisterFlagCompletion(addCmd, "labels-ttl", noFileCompletion)
	mustRegisterFlagCompletion(addCmd, "labels-hook", noFileCompletion)

	addCmd.RunE = func(cmd *cobra.Command, args []string) error {
		effectiveRegions, err := resolveAddRegionFilters(addRegions, addBundles)
		if err != nil {
			return err
		}
		normalizedColor, err := nglstate.NormalizeColorInput(addColor)
		if err != nil {
			return err
		}
		colorByFields, err := resolveAddColorBy(addColorBy, addColorSub)
		if err != nil {
			return err
		}
		if len(colorByFields) > 0 && normalizedColor == "" {
			normalizedColor = "colored"
		}
		if len(colorByFields) > 0 && strings.HasPrefix(normalizedColor, "#") {
			fmt.Fprintln(os.Stderr, "Warning: --color-by with a hex color assigns the same color to every group; use a named palette or 'colored' for distinct group colors")
		}
		fellBack := fallbackNestedColorByFields(colorByFields, normalizedColor)
		if len(fellBack) != len(colorByFields) {
			fmt.Fprintf(os.Stderr, "Warning: --color-by %s,%s needs --color colored to vary hue by %s; coloring by %s alone\n",
				colorByFields[0], colorByFields[1], colorByFields[0], colorByFields[1])
		}
		colorByFields = fellBack
		if addColorSub && normalizedColor == "" {
			fmt.Fprintln(os.Stderr, "Warning: --color-sub has no effect without --color")
			addColorSub = false
		}
		if addColorSub && strings.HasPrefix(normalizedColor, "#") {
			fmt.Fprintln(os.Stderr, "Warning: --color-sub has no effect with a hex color; use a named color or 'colored'")
			addColorSub = false
		}

		baseFilters := &seatable.Filters{
			SuperClass: addSuperClass,
			Side:       addSide,
			Regions:    effectiveRegions,
			Tract:      addTract,
			Proofread:  addProofread,
		}

		hasGroupFlags := len(addCellClasses) > 0 || len(addCellTypes) > 0 || len(addCellSubtypes) > 0
		if err := validateAddInputs(baseFilters, hasGroupFlags); err != nil {
			return err
		}
		if addIntersect && groupDimensionCount(addCellClasses, addCellTypes, addCellSubtypes) < 2 {
			fmt.Fprintln(os.Stderr, "Warning: --intersect has no effect unless you combine two or more of --cell-class/--cell-type/--cell-subtype")
		}

		// Query SeaTable
		client, err := addNewClient()
		if err != nil {
			return err
		}

		// Union (each value its own group) is the default; --intersect opts into
		// the cross-product (AND). buildQuerySpecs takes a union flag.
		specs := buildQuerySpecs(baseFilters, addCellClasses, addCellTypes, addCellSubtypes, !addIntersect)
		if len(colorByFields) > 0 && colorByFields[0] == queryGroupColorByField && len(specs) == 1 {
			fmt.Fprintln(os.Stderr, "Warning: --color-by group found a single query group; repeat --cell-class/--cell-type/--cell-subtype to form groups worth separating")
		}

		var groups [][]string
		var allRootIDs []string
		var allRows []seatable.NeuronRow
		var subtypeMap map[string]string
		if addColorSub {
			subtypeMap = make(map[string]string)
		}

		for _, s := range specs {
			rows, err := addQueryNeurons(client, &s.filters)
			if err != nil {
				return err
			}
			var ids []string
			if addColorSub {
				var sm map[string]string
				ids, sm = extractRootIDsWithSubtype(rows)
				for k, v := range sm {
					subtypeMap[k] = v
				}
			} else {
				ids = extractRootIDs(rows)
			}
			groups = append(groups, ids)
			allRows = append(allRows, rows...)
			if s.label != "" {
				fmt.Fprintf(os.Stderr, "  %s: %d neurons (%d with root IDs)\n", s.label, len(rows), len(ids))
			}
		}

		// Union groups (the default) can overlap when one predicate includes
		// another (a cell_class and one of its own cell types, say), so the same
		// neuron may appear in several groups. Collapse to a unique set by root
		// ID before counting, coloring, output, and injection -- --replace in
		// particular bypasses AddSegments' append-mode dedupe.
		groups, allRootIDs, allRows = dedupeUnionResults(groups, allRows)
		totalRows := len(allRows)

		// --color-by rebinds groups to its own partition, so keep the query
		// groups and their labels for --color-by group to build from.
		queryGroups := groups
		queryLabels := querySpecLabels(specs)

		var nestedGroups [][][]string
		var gradient *gradientColorBy
		switch {
		case len(colorByFields) == 1 && continuousAddColorByFields[colorByFields[0]]:
			// A continuous field has one value per neuron, so report its span
			// rather than a line for every distinct value.
			built := buildGradientColorByValues(allRows, colorByFields[0])
			gradient = &built
			if len(built.values) == 0 {
				fmt.Fprintf(os.Stderr, "  %s: no neuron carries a position; all take the unset color\n", colorByFields[0])
			} else {
				fmt.Fprintf(os.Stderr, "  %s: %d with root IDs, %g to %g\n", colorByFields[0], len(built.values), built.low, built.high)
			}
			if len(built.unset) > 0 {
				fmt.Fprintf(os.Stderr, "  %s=(unset): %d without a position\n", colorByFields[0], len(built.unset))
			}
		case len(colorByFields) == 1:
			var labels []string
			groups, labels = colorByGroupsFromPartitions(
				colorByOuterPartitions(allRows, colorByFields[0], queryGroups, queryLabels))
			reportColorByGroups(os.Stderr, colorByFields, groups, labels)
		case len(colorByFields) == maxAddColorByFields:
			var labels [][]string
			nestedGroups, labels = nestColorByPartitions(
				colorByOuterPartitions(allRows, colorByFields[0], queryGroups, queryLabels), colorByFields[1])
			reportNestedColorByGroups(os.Stderr, colorByFields, nestedGroups, labels)
		}
		// After the groups that were colored, so the two read together.
		if len(colorByFields) > 0 && colorByFields[0] == queryGroupColorByField {
			reportUncoloredQueryGroups(os.Stderr, queryGroups, queryLabels)
		}

		fmt.Fprintf(os.Stderr, "Found %d neurons (%d with root IDs)\n", totalRows, len(allRootIDs))

		if len(allRootIDs) == 0 {
			fmt.Fprintln(os.Stderr, "No neurons found matching filters")
			return nil
		}

		// Root IDs only mode
		if addRootIDsOnly {
			return deliverRootIDs(allRootIDs, cmd.OutOrStdout(), cmd.ErrOrStderr(), addClipboardWrite)
		}

		// Load state
		result, err := addLoadState(addState, addGenerate)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "State loaded from %s\n", result.Source)

		// Find segmentation layer and inject
		layer, _, err := addFindSegmentationLayer(result.State, addLayer)
		if err != nil {
			return err
		}

		nglstate.AddSegments(layer, allRootIDs, addReplace)

		applyAddSegmentColors(layer, addColorPlan{
			rootIDs:       allRootIDs,
			groups:        groups,
			nestedGroups:  nestedGroups,
			gradient:      gradient,
			subtypeMap:    subtypeMap,
			color:         normalizedColor,
			colorByFields: colorByFields,
			colorSub:      addColorSub,
		})

		if addLabels {
			if err := attachCellTypeLabels(layer, allRows, addLabelsTTL, resolveLabelsHook(addLabelsHook)); err != nil {
				return err
			}
		}

		// Deliver file/stdout/clipboard output and the browser request as
		// independent actions so one desktop failure cannot suppress another.
		return addDeliverState(result, nglstate.DeliveryOptions{
			OutputFile: addOutput,
			Open:       addOpen,
		})
	}

	rootCmd.AddCommand(addCmd)
}

func deliverRootIDs(rootIDs []string, stdout, stderr io.Writer, copyText func(string) (clipboard.WriteResult, error)) error {
	joined := strings.Join(rootIDs, "\n")
	fmt.Fprintln(stdout, joined)
	copyResult, err := copyText(joined)
	if err != nil {
		// The IDs already reached stdout, so this command succeeded. Failing
		// here would break piping on headless and CI machines.
		fmt.Fprintf(stderr, "Warning: root IDs were printed, but clipboard copy failed: %v\n", err)
		return nil
	}
	fmt.Fprintf(stderr, "Clipboard: copied root IDs via %s\n", copyResult.Backend)
	return nil
}

func extractRootIDs(rows []seatable.NeuronRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.RootID != "" {
			ids = append(ids, r.RootID)
		}
	}
	return ids
}

func extractRootIDsWithSubtype(rows []seatable.NeuronRow) ([]string, map[string]string) {
	ids := make([]string, 0, len(rows))
	subtypeMap := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.RootID != "" {
			ids = append(ids, r.RootID)
			subtypeMap[r.RootID] = r.CellSubtype
		}
	}
	return ids, subtypeMap
}

// dedupeUnionResults collapses overlapping query groups into a unique set by
// root ID, keeping the first occurrence. Union grouping (the default) can match
// the same neuron under several predicates -- e.g. a cell_class and one of its
// own cell types -- so without this, root IDs repeat across groups and inflate
// counts, --root-ids-only output, and --replace injection (AddSegments only
// dedupes in append mode). The returned groups partition the unique root IDs so
// per-group coloring stays consistent with the injected set. Rows without a
// root ID carry no identity and are passed through unchanged.
func dedupeUnionResults(groups [][]string, rows []seatable.NeuronRow) ([][]string, []string, []seatable.NeuronRow) {
	seenID := make(map[string]bool)
	dedupedGroups := make([][]string, len(groups))
	var allRootIDs []string
	for i, group := range groups {
		kept := make([]string, 0, len(group))
		for _, id := range group {
			if id == "" || seenID[id] {
				continue
			}
			seenID[id] = true
			kept = append(kept, id)
			allRootIDs = append(allRootIDs, id)
		}
		dedupedGroups[i] = kept
	}

	seenRow := make(map[string]bool)
	dedupedRows := make([]seatable.NeuronRow, 0, len(rows))
	for _, r := range rows {
		if r.RootID != "" {
			if seenRow[r.RootID] {
				continue
			}
			seenRow[r.RootID] = true
		}
		dedupedRows = append(dedupedRows, r)
	}

	return dedupedGroups, allRootIDs, dedupedRows
}

func resolveAddRegionFilters(regions, bundles []string) ([]string, error) {
	regions = compactAddValues(regions)
	bundles = compactAddValues(bundles)
	if len(regions) > 0 && len(bundles) > 0 {
		return nil, fmt.Errorf("--region and --bundle cannot be used together")
	}
	if len(bundles) > 0 {
		return bundles, nil
	}
	return regions, nil
}

func compactAddValues(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// resolveAddColorBy parses --color-by into its ordered fields. One field colors
// one group per distinct value. Two comma-separated fields nest: the first picks
// each group's palette family (hue), the second the tone within that family.
func resolveAddColorBy(colorBy string, colorSub bool) ([]string, error) {
	colorBy = strings.TrimSpace(colorBy)
	if colorBy != "" && colorSub {
		return nil, fmt.Errorf("--color-by and --color-sub cannot be used together")
	}
	if colorSub || colorBy == "" {
		return nil, nil
	}

	parts := strings.Split(colorBy, ",")
	if len(parts) > maxAddColorByFields {
		return nil, fmt.Errorf("invalid --color-by %q: at most %d comma-separated fields (hue,tone), got %d", colorBy, maxAddColorByFields, len(parts))
	}

	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			return nil, fmt.Errorf("invalid --color-by %q: empty field", colorBy)
		}
		if !validAddColorByFields[field] {
			return nil, fmt.Errorf("invalid --color-by %q; valid fields: %s", field, addColorByFieldList)
		}
		if slices.Contains(fields, field) {
			return nil, fmt.Errorf("invalid --color-by %q: %q is repeated", colorBy, field)
		}
		fields = append(fields, field)
	}

	// A continuous field spreads segments along a ramp rather than splitting
	// them into the groups a level needs, so it can only stand alone.
	if len(fields) == maxAddColorByFields {
		for _, field := range fields {
			if continuousAddColorByFields[field] {
				return nil, fmt.Errorf("invalid --color-by %q: %q is continuous and cannot be combined with another field", colorBy, field)
			}
		}
		// The query groups are the outer level by construction: they partition
		// the result, and the second field then splits each one.
		if fields[1] == queryGroupColorByField {
			return nil, fmt.Errorf("invalid --color-by %q: %q names the query a neuron matched, so it can only be the first field", colorBy, queryGroupColorByField)
		}
	}
	return fields, nil
}

// fallbackNestedColorByFields drops the outer field when two-level hue/tone
// cannot run. A named palette or hex has only one family, so grouping uses
// the inner field alone.
func fallbackNestedColorByFields(fields []string, color string) []string {
	if len(fields) == 2 && color != "colored" {
		return fields[1:]
	}
	return fields
}

func validateAddOptions(regions, bundles []string, color, colorBy string, colorSub bool) error {
	if _, err := resolveAddRegionFilters(regions, bundles); err != nil {
		return err
	}
	if _, err := nglstate.NormalizeColorInput(color); err != nil {
		return err
	}
	if _, err := resolveAddColorBy(colorBy, colorSub); err != nil {
		return err
	}
	return nil
}

// maxAddColorByFields caps --color-by at the two levels a palette family can
// show: one hue per outer value, one tone per inner value.
const maxAddColorByFields = 2

// addColorByFields lists the --color-by fields in help and error display order.
// "column" is derived from cell_instance; the rest are CRANTb_meta columns.
var addColorByFields = []string{
	"super_class",
	"cell_class",
	"cell_type",
	"cell_subtype",
	"cell_instance",
	"column",
	"side",
	"region",
	"tract",
	"nerve",
	"hemilineage",
	"proofread",
	"pos_x",
	"pos_y",
	"pos_z",
	"root_id",
	"group",
}

// queryGroupColorByField names the query group a neuron matched rather than a
// value on the row, so it forms the outer level from the query itself. It is
// what lets a two-field --color-by reproduce query groups that no single
// metadata field describes: a mixed union, or an --intersect cross-product.
const queryGroupColorByField = "group"

// continuousAddColorByFields are the --color-by fields holding a number rather
// than a category, so they spread segments along a ramp instead of grouping
// them by value.
var continuousAddColorByFields = fieldSet([]string{"pos_x", "pos_y", "pos_z"})

var validAddColorByFields = fieldSet(addColorByFields)

// addColorByFieldList renders addColorByFields for help text and errors.
var addColorByFieldList = strings.Join(addColorByFields, ", ")

func fieldSet(fields []string) map[string]bool {
	set := make(map[string]bool, len(fields))
	for _, field := range fields {
		set[field] = true
	}
	return set
}

func mustMarkFlagDeprecated(cmd *cobra.Command, flagName, usageMessage string) {
	if err := cmd.Flags().MarkDeprecated(flagName, usageMessage); err != nil {
		panic(fmt.Sprintf("deprecating %s --%s: %v", cmd.Name(), flagName, err))
	}
}

func validateAddInputs(baseFilters *seatable.Filters, hasGroupFlags bool) error {
	if !baseFilters.HasAny() && !hasGroupFlags {
		return fmt.Errorf("at least one filter flag is required (e.g. --cell-class, --super-class, --cell-type)")
	}
	return nil
}

type querySpec struct {
	label   string
	filters seatable.Filters
}

// querySpecLabels lists the query group labels in spec order, so they stay
// aligned with the groups built alongside them.
func querySpecLabels(specs []querySpec) []string {
	labels := make([]string, 0, len(specs))
	for _, s := range specs {
		labels = append(labels, s.label)
	}
	return labels
}

// specDim is one grouping dimension (cell_class, cell_type, or cell_subtype)
// contributing its values to the query specs.
type specDim struct {
	values []string
	apply  func(*seatable.Filters, string)
}

// groupDimensionCount reports how many of the cell-class/cell-type/cell-subtype
// dimensions carry at least one value. --intersect only changes the result when
// two or more dimensions are combined.
func groupDimensionCount(cellClasses, cellTypes, cellSubtypes []string) int {
	count := 0
	for _, values := range [][]string{cellClasses, cellTypes, cellSubtypes} {
		if len(values) > 0 {
			count++
		}
	}
	return count
}

// buildQuerySpecs turns the cell-class/cell-type/cell-subtype flags into query
// groups. With union set (the CLI default), every value becomes its own group
// (OR across dimensions), so values from different columns load together as a
// combined set. Without union (--intersect) the non-empty dimensions are
// combined as a cross-product instead (AND within each group: e.g. class
// ER × type ER yields one "ER/ER" group requiring both). Base
// scalar filters (super_class, side, region, ...) are preserved on every group.
// With no grouping values, returns a single group using the base filters.
func buildQuerySpecs(base *seatable.Filters, cellClasses, cellTypes, cellSubtypes []string, union bool) []querySpec {
	dims := []specDim{
		{cellClasses, func(f *seatable.Filters, v string) { f.CellClass = v }},
		{cellTypes, func(f *seatable.Filters, v string) { f.CellType = v }},
		{cellSubtypes, func(f *seatable.Filters, v string) { f.CellSubtype = v }},
	}

	var specs []querySpec
	if union {
		specs = buildUnionSpecs(base, dims)
	} else {
		specs = buildCrossProductSpecs(base, dims)
	}

	if len(specs) == 0 {
		specs = append(specs, querySpec{label: "", filters: *base})
	}
	return specs
}

// buildUnionSpecs makes each dimension value its own group (OR semantics).
func buildUnionSpecs(base *seatable.Filters, dims []specDim) []querySpec {
	var specs []querySpec
	for _, d := range dims {
		for _, v := range d.values {
			f := *base
			d.apply(&f, v)
			specs = append(specs, querySpec{label: v, filters: f})
		}
	}
	return specs
}

// buildCrossProductSpecs expands the non-empty dimensions into their
// cross-product (AND within each group), labelling groups "value1/value2/...".
func buildCrossProductSpecs(base *seatable.Filters, dims []specDim) []querySpec {
	type partial struct {
		labels  []string
		filters seatable.Filters
	}

	partials := []partial{{filters: *base}}
	for _, d := range dims {
		if len(d.values) == 0 {
			continue
		}
		next := make([]partial, 0, len(partials)*len(d.values))
		for _, p := range partials {
			for _, v := range d.values {
				f := p.filters
				d.apply(&f, v)
				labels := append(append([]string{}, p.labels...), v)
				next = append(next, partial{labels: labels, filters: f})
			}
		}
		partials = next
	}

	// The seed partial (no dimensions applied) carries no labels; return no
	// specs so the caller falls back to a single base-only group.
	if len(partials) == 1 && len(partials[0].labels) == 0 {
		return nil
	}

	specs := make([]querySpec, 0, len(partials))
	for _, p := range partials {
		specs = append(specs, querySpec{label: strings.Join(p.labels, "/"), filters: p.filters})
	}
	return specs
}

// addColorPlan carries everything applyAddSegmentColors needs: the segments to
// color, grouped the way the user asked for them, and the resolved color input.
type addColorPlan struct {
	rootIDs       []string     // every injected root ID, for whole-set coloring
	groups        [][]string   // query groups, or one group per single --color-by value
	nestedGroups  [][][]string // set only for a two-field --color-by
	gradient      *gradientColorBy
	subtypeMap    map[string]string
	color         string
	colorByFields []string
	colorSub      bool
}

func applyAddSegmentColors(layer map[string]interface{}, plan addColorPlan) {
	if len(plan.colorByFields) > 0 && plan.color != "" {
		switch {
		case plan.gradient != nil:
			nglstate.SetSegmentColorByGradient(layer, plan.gradient.values, plan.gradient.unset, plan.color)
		case len(plan.colorByFields) == maxAddColorByFields:
			// fallbackNestedColorByFields has already dropped the outer field
			// unless the color is "colored", the one input two levels need.
			nglstate.SetSegmentColorByNestedGroupValues(layer, plan.nestedGroups)
		default:
			nglstate.SetSegmentColorByGroupValues(layer, plan.groups, plan.color)
		}
		return
	}

	// Repeated class/type flags and --color-sub need group-aware base coloring.
	if (len(plan.groups) > 1 || plan.colorSub) && plan.color != "" {
		nglstate.SetSegmentColorByGroups(layer, plan.groups, plan.color)
	} else {
		nglstate.SetSegmentColor(layer, plan.rootIDs, plan.color)
	}
	if plan.colorSub {
		nglstate.SetSegmentColorBySubtype(layer, plan.groups, plan.subtypeMap, plan.color)
	}
}

// attachCellTypeLabels publishes the queried neurons' cell types as a
// segment-properties source (a secret gist, or via a publish hook when hookCmd
// is set) and attaches it to the layer, so types render next to root IDs in the
// Seg. panel. Prior label sources are cleaned up (older than ttl) and replaced
// rather than accumulated.
func attachCellTypeLabels(layer map[string]interface{}, rows []seatable.NeuronRow, ttl time.Duration, hookCmd string) error {
	if hookCmd == "" {
		if err := labelhost.EnsureGistAvailable(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Note: --labels publishes the queried root IDs and their cell types/tags to an unlisted GitHub gist; it is reachable by anyone who has the resulting state URL.")
	} else {
		fmt.Fprintf(os.Stderr, "Publishing labels via hook: %s\n", hookCmd)
	}

	info, err := segprops.BuildSegmentProperties(rows, segprops.DefaultOptions())
	if err != nil {
		return fmt.Errorf("building segment properties: %w", err)
	}

	prior := labelhost.RecordedURLs()
	if err := labelhost.GC(ttl, hookCmd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: label cleanup failed: %v\n", err)
	}

	pub, err := labelhost.Publish(hookCmd, info)
	if err != nil {
		return fmt.Errorf("publishing labels: %w", err)
	}
	if err := labelhost.Record(pub); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not record label source for cleanup: %v\n", err)
	}

	if err := nglstate.EnsureSegmentPropertiesSource(layer, pub.URL, prior); err != nil {
		return fmt.Errorf("attaching label source: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Attached cell-type labels (%s %s)\n", pub.Kind, pub.ID)
	return nil
}

// colorByPartition is one formed color-by group: the label describing it and
// the rows it holds. A single --color-by colors the partitions themselves; a
// two-field one splits each partition again by the second field.
type colorByPartition struct {
	label string
	rows  []seatable.NeuronRow
}

// colorByOuterPartitions forms the outer level. Every field but "group" reads a
// value off each row; "group" instead reuses the query groups, which is the one
// partition no field can describe.
func colorByOuterPartitions(rows []seatable.NeuronRow, field string, queryGroups [][]string, queryLabels []string) []colorByPartition {
	if field == queryGroupColorByField {
		return partitionRowsByQueryGroup(rows, queryGroups, queryLabels)
	}
	return partitionRowsByField(rows, field)
}

// partitionRowsByField collects the rows sharing each value of a field, in
// ascending value order with the empty value last.
func partitionRowsByField(rows []seatable.NeuronRow, field string) []colorByPartition {
	rowsByValue := make(map[string][]seatable.NeuronRow)
	var values []string

	for _, row := range rows {
		if row.RootID == "" {
			continue
		}
		value := addColorByFieldValue(row, field)
		if _, ok := rowsByValue[value]; !ok {
			values = append(values, value)
		}
		rowsByValue[value] = append(rowsByValue[value], row)
	}
	sortColorByValues(values)

	partitions := make([]colorByPartition, 0, len(values))
	for _, value := range values {
		partitions = append(partitions, colorByPartition{label: colorByLabel(field, value), rows: rowsByValue[value]})
	}
	return partitions
}

// partitionRowsByQueryGroup rebuilds the query groups as partitions. After
// dedupeUnionResults, queryGroups[i] holds exactly the root IDs spec i owns and
// queryLabels[i] names that spec, so a row belongs to the group holding its
// root ID. Groups left empty by deduplication are dropped rather than handed a
// palette family that would color nothing.
func partitionRowsByQueryGroup(rows []seatable.NeuronRow, queryGroups [][]string, queryLabels []string) []colorByPartition {
	owner := make(map[string]int)
	for i, group := range queryGroups {
		for _, rootID := range group {
			owner[rootID] = i
		}
	}

	rowsByGroup := make([][]seatable.NeuronRow, len(queryGroups))
	for _, row := range rows {
		if row.RootID == "" {
			continue
		}
		i, ok := owner[row.RootID]
		if !ok {
			continue
		}
		rowsByGroup[i] = append(rowsByGroup[i], row)
	}

	partitions := make([]colorByPartition, 0, len(queryGroups))
	for i, groupRows := range rowsByGroup {
		if len(groupRows) == 0 {
			continue
		}
		partitions = append(partitions, colorByPartition{
			label: queryGroupLabel(queryLabels[i]),
			rows:  groupRows,
		})
	}
	return partitions
}

// queryGroupLabel names a query group. A query with no repeated classifier
// flags forms one unnamed group holding the whole result, which is not the
// missing value "(empty)" stands for everywhere else.
func queryGroupLabel(specLabel string) string {
	if specLabel == "" {
		specLabel = "(all)"
	}
	return colorByLabel(queryGroupColorByField, specLabel)
}

// reportUncoloredQueryGroups names the query groups --color-by group leaves
// out. A group holding no root IDs of its own -- because it matched nothing, or
// because an earlier group already claimed every neuron it found -- takes no
// color family, which also spaces the remaining groups as though it never
// existed. Saying so beats dropping it silently. A result where no group holds
// anything has no coloring for a group to be left out of, so it reports
// nothing.
func reportUncoloredQueryGroups(w io.Writer, queryGroups [][]string, queryLabels []string) {
	if !slices.ContainsFunc(queryGroups, func(group []string) bool { return len(group) > 0 }) {
		return
	}
	for i, group := range queryGroups {
		if len(group) > 0 {
			continue
		}
		fmt.Fprintf(w, "  %s: no root IDs of its own; not colored\n", queryGroupLabel(queryLabels[i]))
	}
}

// colorByGroupsFromPartitions flattens partitions into the root-ID groups the
// coloring functions take, keeping labels aligned.
func colorByGroupsFromPartitions(partitions []colorByPartition) ([][]string, []string) {
	groups := make([][]string, 0, len(partitions))
	labels := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		rootIDs := make([]string, 0, len(partition.rows))
		for _, row := range partition.rows {
			rootIDs = append(rootIDs, row.RootID)
		}
		groups = append(groups, rootIDs)
		labels = append(labels, partition.label)
	}
	return groups, labels
}

// nestColorByPartitions splits every outer partition by the inner field. The
// returned slices share one shape, so labels[i][j] names groups[i][j].
func nestColorByPartitions(partitions []colorByPartition, inner string) ([][][]string, [][]string) {
	groups := make([][][]string, 0, len(partitions))
	labels := make([][]string, 0, len(partitions))
	for _, partition := range partitions {
		family, innerLabels := buildColorByGroups(partition.rows, inner)
		familyLabels := make([]string, 0, len(innerLabels))
		for _, innerLabel := range innerLabels {
			familyLabels = append(familyLabels, partition.label+" / "+innerLabel)
		}
		groups = append(groups, family)
		labels = append(labels, familyLabels)
	}
	return groups, labels
}

func buildColorByGroups(rows []seatable.NeuronRow, field string) ([][]string, []string) {
	return colorByGroupsFromPartitions(partitionRowsByField(rows, field))
}

// buildNestedColorByGroups groups rows by the outer field, then by the inner
// field within each outer group.
func buildNestedColorByGroups(rows []seatable.NeuronRow, outer, inner string) ([][][]string, [][]string) {
	return nestColorByPartitions(partitionRowsByField(rows, outer), inner)
}

// gradientColorBy holds the per-neuron values a continuous --color-by field
// spreads along the ramp, plus the root IDs it has no value for.
type gradientColorBy struct {
	values    map[string]float64
	unset     []string
	low, high float64
}

// buildGradientColorByValues reads a continuous field off every matched row.
// Rows the field has no number for are collected separately so they can be
// marked rather than silently placed somewhere on the ramp.
func buildGradientColorByValues(rows []seatable.NeuronRow, field string) gradientColorBy {
	gradient := gradientColorBy{values: make(map[string]float64, len(rows))}
	first := true

	for _, row := range rows {
		if row.RootID == "" {
			continue
		}
		value, ok := addColorByFieldNumber(row, field)
		if !ok {
			gradient.unset = append(gradient.unset, row.RootID)
			continue
		}
		gradient.values[row.RootID] = value
		if first || value < gradient.low {
			gradient.low = value
		}
		if first || value > gradient.high {
			gradient.high = value
		}
		first = false
	}
	return gradient
}

// addColorByFieldNumber returns the value of a continuous --color-by field and
// whether the row carries one.
func addColorByFieldNumber(row seatable.NeuronRow, field string) (float64, bool) {
	if !row.PositionSet {
		return 0, false
	}
	switch field {
	case "pos_x":
		return row.X, true
	case "pos_y":
		return row.Y, true
	case "pos_z":
		return row.Z, true
	default:
		return 0, false
	}
}

func colorByLabel(field, value string) string {
	if value == "" {
		value = "(empty)"
	}
	return field + "=" + value
}

func sortColorByValues(values []string) {
	sort.Slice(values, func(i, j int) bool {
		if values[i] == "" {
			return false
		}
		if values[j] == "" {
			return true
		}
		return values[i] < values[j]
	})
}

func addColorByFieldValue(row seatable.NeuronRow, field string) string {
	switch field {
	case "column":
		return columnFromCellInstance(row.CellInstance)
	case "root_id":
		return row.RootID
	default:
		return seatable.FieldValue(row, field)
	}
}

// colorByGroupsAreNeurons reports whether a field gives every neuron a group of
// its own, which turns a line-per-group report into a line per neuron.
func colorByGroupsAreNeurons(fields []string) bool {
	return slices.Contains(fields, "root_id")
}

// reportColorByGroups prints how many root IDs each color-by group holds.
func reportColorByGroups(w io.Writer, fields []string, groups [][]string, labels []string) {
	if colorByGroupsAreNeurons(fields) {
		fmt.Fprintf(w, "  %s: %d neurons, one color each\n", strings.Join(fields, ","), len(groups))
		return
	}
	for i, group := range groups {
		fmt.Fprintf(w, "  %s: %d with root IDs\n", labels[i], len(group))
	}
}

// reportNestedColorByGroups does the same for the two-field form, where every
// inner group sits inside one outer group.
func reportNestedColorByGroups(w io.Writer, fields []string, groups [][][]string, labels [][]string) {
	if colorByGroupsAreNeurons(fields) {
		inner := 0
		for _, family := range groups {
			inner += len(family)
		}
		fmt.Fprintf(w, "  %s: %d neurons, one color each\n", strings.Join(fields, ","), inner)
		return
	}
	for i, family := range groups {
		for j, group := range family {
			fmt.Fprintf(w, "  %s: %d with root IDs\n", labels[i][j], len(group))
		}
	}
}

func columnFromCellInstance(instance string) string {
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return ""
	}
	if strings.HasPrefix(instance, "\u03947_") || strings.HasPrefix(strings.ToLower(instance), "delta7_") {
		return suffixRunes(instance, 4)
	}
	return suffixRunes(instance, 2)
}

func suffixRunes(value string, n int) string {
	runes := []rune(value)
	if len(runes) <= n {
		return value
	}
	return string(runes[len(runes)-n:])
}
