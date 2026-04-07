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

  # Add all neurons annotated to bundle/region LX
  crantcli add --bundle LX

  # Mix cell classes and types as independent groups
  crantcli add --cell-class kenyon_cell --cell-type ER --color colored`,
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
		addReplace     bool
		addRootIDsOnly bool
		addOpen        bool
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
	addCmd.Flags().StringVar(&addColor, "color", "", "Segment color: named (blue, red, green, turquoise) with auto-toning, 'colored' for random, or hex (#ff0000)")
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

		// Build independent query groups: each --cell-class and --cell-type
		// is its own group so they can be freely mixed.
		type querySpec struct {
			label   string
			filters seatable.Filters
		}
		var specs []querySpec

		for _, cc := range addCellClasses {
			f := *baseFilters
			f.CellClass = cc
			specs = append(specs, querySpec{label: cc, filters: f})
		}
		for _, ct := range addCellTypes {
			f := *baseFilters
			f.CellType = ct
			specs = append(specs, querySpec{label: ct, filters: f})
		}

		// If no class/type flags were given, run a single query with base filters.
		if len(specs) == 0 {
			specs = append(specs, querySpec{label: "", filters: *baseFilters})
		}

		var groups [][]string
		var allRootIDs []string
		var totalRows int

		for _, s := range specs {
			rows, err := seatable.QueryNeurons(client, &s.filters)
			if err != nil {
				return err
			}
			ids := extractRootIDs(rows)
			groups = append(groups, ids)
			allRootIDs = append(allRootIDs, ids...)
			totalRows += len(rows)
			if s.label != "" {
				fmt.Fprintf(os.Stderr, "  %s: %d neurons (%d with root IDs)\n", s.label, len(rows), len(ids))
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

		// Color assignment: per-group palette toning for multi-group, single resolve otherwise
		if len(groups) > 1 && normalizedColor != "" {
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
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.RootID != "" {
			ids = append(ids, r.RootID)
		}
	}
	return ids
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

func validateAddInputs(baseFilters *seatable.Filters, hasGroupFlags bool) error {
	if !baseFilters.HasAny() && !hasGroupFlags {
		return fmt.Errorf("at least one filter flag is required (e.g. --cell-class, --super-class, --cell-type)")
	}
	return nil
}
