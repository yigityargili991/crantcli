package cmd

import (
	"fmt"
	"os"
	"strings"

	"crantcli/internal/browser"
	"crantcli/internal/clipboard"
	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Query neurons and inject root IDs into a Neuroglancer state",
	Long: `Query the CRANT dataset for neurons matching the given filters and inject
their root IDs into a Neuroglancer state.

Smart input resolution (when no --state is given):
  1. Check stdin for piped JSON
  2. Check clipboard for a Neuroglancer URL
  3. Check last state URL produced by this tool
  4. Fall back to the default CRANT scene template

Examples:
  # Smart: checks clipboard for Neuroglancer URL, injects, copies back
  crantcli add --cell-class kenyon_cell

  # Explicit file I/O
  crantcli add --cell-class kenyon_cell -s state.json -o modified.json

  # Generate fresh state
  crantcli add --cell-class kenyon_cell --generate

  # Open updated state in browser
  crantcli add --cell-type ER --open

  # Just get root IDs (no state manipulation)
  crantcli add --cell-class kenyon_cell --root-ids-only

  # Add multiple cell types with per-group coloring
  crantcli add --cell-type ER --cell-type EPG/PEG --color colored

  # Add sensory neurons and color by cell_type
  crantcli add --super-class sensory --color-by cell_type

  # Add all neurons annotated to bundle/region LX
  crantcli add --bundle LX

  # Cross-product: kenyon_cell+ER and kenyon_cell+EPG/PEG as separate groups
  crantcli add --cell-class kenyon_cell --cell-type ER --cell-type EPG/PEG --color colored

  # Backward-compatible shorthand for --color-by cell_subtype
  crantcli add --cell-class kenyon_cell --color-sub --color blue`,
	Annotations: map[string]string{"requiresToken": "true"},
}

func init() {
	var (
		addSuperClass  string
		addCellClasses []string
		addCellTypes   []string
		addCellSubtype string
		addSide        string
		addRegion      string
		addBundle      string
		addTract       string
		addProofread   string
		addState       string
		addGenerate    bool
		addOutput      string
		addLayer       string
		addColor       string
		addColorBy     string
		addReplace     bool
		addRootIDsOnly bool
		addOpen        bool
		addColorSub    bool
	)

	addCmd.Flags().StringVar(&addSuperClass, "super-class", "", "Filter by super_class")
	addCmd.Flags().StringArrayVar(&addCellClasses, "cell-class", nil, "Filter by cell_class (repeatable for multiple classes)")
	addCmd.Flags().StringArrayVar(&addCellTypes, "cell-type", nil, "Filter by cell_type (repeatable for multiple types)")
	addCmd.Flags().StringVar(&addCellSubtype, "cell-subtype", "", "Filter by cell_subtype")
	addCmd.Flags().StringVar(&addSide, "side", "", "Filter by side")
	addCmd.Flags().StringVar(&addRegion, "region", "", "Filter by region")
	addCmd.Flags().StringVar(&addBundle, "bundle", "", "Filter by bundle region annotation (alias of --region, e.g. LX)")
	addCmd.Flags().StringVar(&addTract, "tract", "", "Filter by tract")
	addCmd.Flags().StringVar(&addProofread, "proofread", "", "Filter by proofread status")
	addCmd.Flags().StringVarP(&addState, "state", "s", "", "Neuroglancer state (URL or file path)")
	addCmd.Flags().BoolVarP(&addGenerate, "generate", "g", false, "Generate from default template instead of clipboard/session state")
	addCmd.Flags().StringVarP(&addOutput, "output", "o", "", "Output file path (default: clipboard or stdout)")
	addCmd.Flags().StringVarP(&addLayer, "layer", "l", "", "Target segmentation layer name")
	addCmd.Flags().StringVar(&addColor, "color", "", "Segment color: named (blue, red, green, turquoise, orange, purple, yellow, pink, brown, indigo, teal, lime) with auto-toning, 'colored' for per-group palette cycling, or hex (#ff0000)")
	addCmd.Flags().StringVar(&addColorBy, "color-by", "", "Color matched rows by field: super_class, cell_class, cell_type, cell_subtype, side, region, tract, nerve, hemilineage, proofread")
	addCmd.Flags().BoolVar(&addColorSub, "color-sub", false, "Backward-compatible shorthand for --color-by cell_subtype")
	addCmd.Flags().BoolVar(&addReplace, "replace", false, "Replace existing segments instead of appending")
	addCmd.Flags().BoolVar(&addRootIDsOnly, "root-ids-only", false, "Just print root IDs, no state manipulation")
	addCmd.Flags().BoolVar(&addOpen, "open", false, "Open updated Neuroglancer URL in default browser")

	addCmd.RunE = func(cmd *cobra.Command, args []string) error {
		effectiveRegion, err := resolveAddRegionFilter(addRegion, addBundle)
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

		baseFilters := &seatable.Filters{
			SuperClass:  addSuperClass,
			CellSubtype: addCellSubtype,
			Side:        addSide,
			Region:      effectiveRegion,
			Tract:       addTract,
			Proofread:   addProofread,
		}

		hasGroupFlags := len(addCellClasses) > 0 || len(addCellTypes) > 0
		if err := validateAddInputs(baseFilters, hasGroupFlags); err != nil {
			return err
		}

		// Query SeaTable
		client, err := seatable.NewClient()
		if err != nil {
			return err
		}

		specs := buildQuerySpecs(baseFilters, addCellClasses, addCellTypes)

		var groups [][]string
		var allRootIDs []string
		var totalRows int
		var allRows []seatable.NeuronRow

		for _, s := range specs {
			rows, err := seatable.QueryNeurons(client, &s.filters)
			if err != nil {
				return err
			}
			ids := extractRootIDs(rows)
			groups = append(groups, ids)
			allRootIDs = append(allRootIDs, ids...)
			allRows = append(allRows, rows...)
			totalRows += len(rows)
			if s.label != "" {
				fmt.Fprintf(os.Stderr, "  %s: %d neurons (%d with root IDs)\n", s.label, len(rows), len(ids))
			}
		}

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

		// Color assignment: repeated class/type flags and --color-by both use
		// group-aware coloring. For --color-by, keep group-aware coloring even
		// when only one field value is present so "colored" uses a palette.
		if (len(groups) > 1 || colorByField != "") && normalizedColor != "" {
			nglstate.SetSegmentColorByGroups(layer, groups, normalizedColor)
		} else {
			nglstate.SetSegmentColor(layer, allRootIDs, normalizedColor)
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
	ids, _ := extractRootIDsWithSubtype(rows)
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

func resolveAddRegionFilter(region, bundle string) (string, error) {
	region = strings.TrimSpace(region)
	bundle = strings.TrimSpace(bundle)
	if region != "" && bundle != "" {
		return "", fmt.Errorf("--region and --bundle cannot be used together")
	}
	if bundle != "" {
		return bundle, nil
	}
	return region, nil
}

func resolveAddColorBy(colorBy string, colorSub bool) (string, error) {
	colorBy = strings.TrimSpace(colorBy)
	if colorBy != "" && colorSub {
		return "", fmt.Errorf("--color-by and --color-sub cannot be used together; use --color-by cell_subtype")
	}
	if colorSub {
		return "cell_subtype", nil
	}
	if colorBy == "" {
		return "", nil
	}
	if !validAddColorByFields[colorBy] {
		return "", fmt.Errorf("invalid --color-by %q; valid fields: super_class, cell_class, cell_type, cell_subtype, side, region, tract, nerve, hemilineage, proofread", colorBy)
	}
	return colorBy, nil
}

var validAddColorByFields = map[string]bool{
	"super_class":  true,
	"cell_class":   true,
	"cell_type":    true,
	"cell_subtype": true,
	"side":         true,
	"region":       true,
	"tract":        true,
	"nerve":        true,
	"hemilineage":  true,
	"proofread":    true,
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

// buildQuerySpecs creates query groups from cell-class and cell-type flags.
// When both are given, produces the cross-product. When only one dimension
// is given, each value is its own group. With neither, returns a single
// group using the base filters.
func buildQuerySpecs(base *seatable.Filters, cellClasses, cellTypes []string) []querySpec {
	var specs []querySpec

	if len(cellClasses) > 0 && len(cellTypes) > 0 {
		for _, cc := range cellClasses {
			for _, ct := range cellTypes {
				f := *base
				f.CellClass = cc
				f.CellType = ct
				specs = append(specs, querySpec{label: cc + "/" + ct, filters: f})
			}
		}
	} else {
		for _, cc := range cellClasses {
			f := *base
			f.CellClass = cc
			specs = append(specs, querySpec{label: cc, filters: f})
		}
		for _, ct := range cellTypes {
			f := *base
			f.CellType = ct
			specs = append(specs, querySpec{label: ct, filters: f})
		}
	}

	if len(specs) == 0 {
		specs = append(specs, querySpec{label: "", filters: *base})
	}
	return specs
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

func addColorByFieldValue(row seatable.NeuronRow, field string) string {
	switch field {
	case "super_class":
		return row.SuperClass
	case "cell_class":
		return row.CellClass
	case "cell_type":
		return row.CellType
	case "cell_subtype":
		return row.CellSubtype
	case "side":
		return row.Side
	case "region":
		return row.Region
	case "tract":
		return row.Tract
	case "nerve":
		return row.Nerve
	case "hemilineage":
		return row.Hemilineage
	case "proofread":
		return row.Proofread
	default:
		return ""
	}
}
