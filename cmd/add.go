package cmd

import (
	"fmt"
	"os"
	"strings"

	"crantinject/internal/browser"
	"crantinject/internal/clipboard"
	"crantinject/internal/nglstate"
	"crantinject/internal/seatable"

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
  crantinject add --cell-class kenyon_cell

  # Explicit file I/O
  crantinject add --cell-class kenyon_cell -s state.json -o modified.json

  # Generate fresh state
  crantinject add --cell-class kenyon_cell --generate

  # Open updated state in browser
  crantinject add --cell-type ER --open

  # Force clipboard overwrite output mode
  crantinject add --cell-class kenyon_cell --pile

  # Just get root IDs (no state manipulation)
  crantinject add --cell-class kenyon_cell --root-ids-only

  # Add multiple cell types with per-type coloring
  crantinject add --cell-type ER --cell-type EPG/PEG --color colored`,
	Annotations: map[string]string{"requiresToken": "true"},
	RunE:        runAdd,
}

var (
	addSuperClass  string
	addCellClass   string
	addCellTypes   []string
	addCellSubtype string
	addSide        string
	addRegion      string
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
	addPile        bool
)

func init() {
	addCmd.Flags().StringVar(&addSuperClass, "super-class", "", "Filter by super_class")
	addCmd.Flags().StringVar(&addCellClass, "cell-class", "", "Filter by cell_class")
	addCmd.Flags().StringArrayVar(&addCellTypes, "cell-type", nil, "Filter by cell_type (repeatable for multiple types)")
	addCmd.Flags().StringVar(&addCellSubtype, "cell-subtype", "", "Filter by cell_subtype")
	addCmd.Flags().StringVar(&addSide, "side", "", "Filter by side")
	addCmd.Flags().StringVar(&addRegion, "region", "", "Filter by region")
	addCmd.Flags().StringVar(&addTract, "tract", "", "Filter by tract")
	addCmd.Flags().StringVar(&addProofread, "proofread", "", "Filter by proofread status")
	addCmd.Flags().StringVarP(&addState, "state", "s", "", "Neuroglancer state (URL or file path)")
	addCmd.Flags().BoolVarP(&addGenerate, "generate", "g", false, "Generate from default template instead of clipboard/session state")
	addCmd.Flags().StringVarP(&addOutput, "output", "o", "", "Output file path (default: clipboard or stdout)")
	addCmd.Flags().StringVarP(&addLayer, "layer", "l", "", "Target segmentation layer name")
	addCmd.Flags().StringVar(&addColor, "color", "", "Segment color: named (blue, red, green, turquoise) with auto-toning, 'colored' for random, or hex (#ff0000)")
	addCmd.Flags().BoolVar(&addReplace, "replace", false, "Replace existing segments instead of appending")
	addCmd.Flags().BoolVar(&addPile, "pile", false, "Force clipboard mode: overwrite clipboard with updated Neuroglancer URL")
	addCmd.Flags().BoolVar(&addRootIDsOnly, "root-ids-only", false, "Just print root IDs, no state manipulation")
	addCmd.Flags().BoolVar(&addOpen, "open", false, "Open updated Neuroglancer URL in default browser")

	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	baseFilters := &seatable.Filters{
		SuperClass:  addSuperClass,
		CellClass:   addCellClass,
		CellSubtype: addCellSubtype,
		Side:        addSide,
		Region:      addRegion,
		Tract:       addTract,
		Proofread:   addProofread,
	}

	if !baseFilters.HasAny() && len(addCellTypes) == 0 {
		return fmt.Errorf("at least one filter flag is required (e.g. --cell-class, --super-class, --cell-type)")
	}
	if addPile && addOutput != "" {
		return fmt.Errorf("--pile cannot be used with --output")
	}
	if addPile && addRootIDsOnly {
		return fmt.Errorf("--pile cannot be used with --root-ids-only")
	}

	// Query SeaTable
	client, err := seatable.NewClient()
	if err != nil {
		return err
	}

	// Query per cell type to track groups, or once if no cell types given.
	var groups [][]string
	var allRootIDs []string
	var totalRows int

	if len(addCellTypes) == 0 {
		rows, err := seatable.QueryNeurons(client, baseFilters)
		if err != nil {
			return err
		}
		ids := extractRootIDs(rows)
		groups = append(groups, ids)
		allRootIDs = ids
		totalRows = len(rows)
	} else {
		for _, ct := range addCellTypes {
			f := *baseFilters
			f.CellType = ct
			rows, err := seatable.QueryNeurons(client, &f)
			if err != nil {
				return err
			}
			ids := extractRootIDs(rows)
			groups = append(groups, ids)
			allRootIDs = append(allRootIDs, ids...)
			totalRows += len(rows)
			fmt.Fprintf(os.Stderr, "  %s: %d neurons (%d with root IDs)\n", ct, len(rows), len(ids))
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

	// Color assignment: per-group palette toning for multi-type, single resolve otherwise
	if len(addCellTypes) > 1 && addColor != "" {
		nglstate.SetSegmentColorByGroups(layer, groups, addColor)
	} else {
		nglstate.SetSegmentColor(layer, allRootIDs, addColor)
	}

	if addPile {
		result.Source = nglstate.SourceClipboard
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

func extractRootIDs(rows []seatable.NeuronRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.RootID != "" {
			ids = append(ids, r.RootID)
		}
	}
	return ids
}
