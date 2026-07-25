package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"crantcli/internal/browser"
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

  # Sub-color by cell_subtype within each query group
  crantcli add --cell-type ER --color-sub --color blue`,
	Annotations: map[string]string{"requiresToken": "true"},
}

func init() {
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
	)

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
	addCmd.Flags().StringVar(&addColorBy, "color-by", "", "Color matched rows by field: super_class, cell_class, cell_type, cell_subtype, cell_instance, column, side, region, tract, nerve, hemilineage, proofread")
	addCmd.Flags().BoolVar(&addColorSub, "color-sub", false, "Sub-color neurons by cell_subtype within each query group")
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
	mustRegisterFlagCompletion(addCmd, "color-by", completeStaticValues(colorByCompletions))
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
		colorByField, err := resolveAddColorBy(addColorBy, addColorSub)
		if err != nil {
			return err
		}
		if colorByField != "" && normalizedColor == "" {
			normalizedColor = "colored"
		}
		if colorByField != "" && strings.HasPrefix(normalizedColor, "#") {
			fmt.Fprintln(os.Stderr, "Warning: --color-by with a hex color assigns the same color to every group; use a named palette or 'colored' for distinct group colors")
		}
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
		client, err := seatable.NewClient()
		if err != nil {
			return err
		}

		// Union (each value its own group) is the default; --intersect opts into
		// the cross-product (AND). buildQuerySpecs takes a union flag.
		specs := buildQuerySpecs(baseFilters, addCellClasses, addCellTypes, addCellSubtypes, !addIntersect)

		var groups [][]string
		var allRootIDs []string
		var allRows []seatable.NeuronRow
		var subtypeMap map[string]string
		if addColorSub {
			subtypeMap = make(map[string]string)
		}

		for _, s := range specs {
			rows, err := seatable.QueryNeurons(client, &s.filters)
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

		if colorByField != "" {
			var labels []string
			groups, labels = buildColorByGroups(allRows, colorByField)
			for i, group := range groups {
				fmt.Fprintf(os.Stderr, "  %s: %d with root IDs\n", labels[i], len(group))
			}
		}

		fmt.Fprintf(os.Stderr, "Found %d neurons (%d with root IDs)\n", totalRows, len(allRootIDs))

		if len(allRootIDs) == 0 {
			fmt.Fprintln(os.Stderr, "No neurons found matching filters")
			return nil
		}

		// Root IDs only mode
		if addRootIDsOnly {
			joined := strings.Join(allRootIDs, "\n")
			fmt.Println(joined)
			if err := clipboard.Write(joined); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not copy to clipboard: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "Root IDs copied to clipboard\n")
			}
			return nil
		}

		// Load state
		result, err := nglstate.LoadState(addState, addGenerate)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "State loaded from %s\n", result.Source)

		// Find segmentation layer and inject
		layer, _, err := nglstate.FindSegmentationLayer(result.State, addLayer)
		if err != nil {
			return err
		}

		nglstate.AddSegments(layer, allRootIDs, addReplace)

		applyAddSegmentColors(layer, allRootIDs, groups, subtypeMap, normalizedColor, colorByField, addColorSub)

		if addLabels {
			if err := attachCellTypeLabels(layer, allRows, addLabelsTTL, resolveLabelsHook(addLabelsHook)); err != nil {
				return err
			}
		}

		// Output
		if err := nglstate.WriteState(result, addOutput); err != nil {
			return err
		}

		if addOpen {
			nglURL := result.OutputURL
			if nglURL == "" {
				var err error
				nglURL, err = nglstate.EncodeURL(result.State, "")
				if err != nil {
					return fmt.Errorf("encoding URL for --open: %w", err)
				}
			}

			if err := browser.OpenURL(nglURL); err != nil {
				return fmt.Errorf("opening browser: %w", err)
			}
			fmt.Fprintln(os.Stderr, "Opened Neuroglancer URL in browser")
		}

		return nil
	}

	rootCmd.AddCommand(addCmd)
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

func resolveAddColorBy(colorBy string, colorSub bool) (string, error) {
	colorBy = strings.TrimSpace(colorBy)
	if colorBy != "" && colorSub {
		return "", fmt.Errorf("--color-by and --color-sub cannot be used together")
	}
	if colorSub {
		return "", nil
	}
	if colorBy == "" {
		return "", nil
	}
	if !validAddColorByFields[colorBy] {
		return "", fmt.Errorf("invalid --color-by %q; valid fields: super_class, cell_class, cell_type, cell_subtype, cell_instance, column, side, region, tract, nerve, hemilineage, proofread", colorBy)
	}
	return colorBy, nil
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

var validAddColorByFields = map[string]bool{
	"super_class":   true,
	"cell_class":    true,
	"cell_type":     true,
	"cell_subtype":  true,
	"cell_instance": true,
	"column":        true,
	"side":          true,
	"region":        true,
	"tract":         true,
	"nerve":         true,
	"hemilineage":   true,
	"proofread":     true,
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

func applyAddSegmentColors(layer map[string]interface{}, allRootIDs []string, groups [][]string, subtypeMap map[string]string, normalizedColor, colorByField string, colorSub bool) {
	if colorByField != "" && normalizedColor != "" {
		nglstate.SetSegmentColorByGroupValues(layer, groups, normalizedColor)
		return
	}

	// Repeated class/type flags and --color-sub need group-aware base coloring.
	if (len(groups) > 1 || colorSub) && normalizedColor != "" {
		nglstate.SetSegmentColorByGroups(layer, groups, normalizedColor)
	} else {
		nglstate.SetSegmentColor(layer, allRootIDs, normalizedColor)
	}
	if colorSub {
		nglstate.SetSegmentColorBySubtype(layer, groups, subtypeMap, normalizedColor)
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

func buildColorByGroups(rows []seatable.NeuronRow, field string) ([][]string, []string) {
	groupsByValue := make(map[string][]string)
	var values []string

	for _, row := range rows {
		if row.RootID == "" {
			continue
		}
		value := addColorByFieldValue(row, field)
		if _, ok := groupsByValue[value]; !ok {
			values = append(values, value)
		}
		groupsByValue[value] = append(groupsByValue[value], row.RootID)
	}
	sortColorByValues(values)

	groups := make([][]string, 0, len(values))
	labels := make([]string, 0, len(values))
	for _, value := range values {
		groups = append(groups, groupsByValue[value])
		labelValue := value
		if labelValue == "" {
			labelValue = "(empty)"
		}
		labels = append(labels, field+"="+labelValue)
	}
	return groups, labels
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
	if field == "column" {
		return columnFromCellInstance(row.CellInstance)
	}
	return seatable.FieldValue(row, field)
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
