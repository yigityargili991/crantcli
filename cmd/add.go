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

Input modes:
  - Default: read Neuroglancer URL from clipboard and append selected neurons
  - --state: use explicit state URL/file
  - --generate: start from default CRANT scene template
  - --unpile: reset to default template, then add selected neurons

Examples:
  # Default: append to Neuroglancer URL currently in clipboard
  crantinject add --cell-class kenyon_cell

  # Explicit file I/O
  crantinject add --cell-class kenyon_cell -s state.json -o modified.json

  # Generate fresh state
  crantinject add --cell-class kenyon_cell --generate

  # Reset from clean template, then add
  crantinject add --cell-class kenyon_cell --unpile

  # Open updated state in browser
  crantinject add --cell-type ER --open

  # Just get root IDs (no state manipulation)
  crantinject add --cell-class kenyon_cell --root-ids-only`,
	Annotations: map[string]string{"requiresToken": "true"},
	RunE:        runAdd,
}

var (
	addSuperClass  string
	addCellClass   string
	addCellType    string
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
	addRootIDsOnly bool
	addOpen        bool
	addUnpile      bool
)

func init() {
	addCmd.Flags().StringVar(&addSuperClass, "super-class", "", "Filter by super_class")
	addCmd.Flags().StringVar(&addCellClass, "cell-class", "", "Filter by cell_class")
	addCmd.Flags().StringVar(&addCellType, "cell-type", "", "Filter by cell_type")
	addCmd.Flags().StringVar(&addCellSubtype, "cell-subtype", "", "Filter by cell_subtype")
	addCmd.Flags().StringVar(&addSide, "side", "", "Filter by side")
	addCmd.Flags().StringVar(&addRegion, "region", "", "Filter by region")
	addCmd.Flags().StringVar(&addTract, "tract", "", "Filter by tract")
	addCmd.Flags().StringVar(&addProofread, "proofread", "", "Filter by proofread status")
	addCmd.Flags().StringVarP(&addState, "state", "s", "", "Neuroglancer state (URL or file path)")
	addCmd.Flags().BoolVarP(&addGenerate, "generate", "g", false, "Generate from default template instead of clipboard state")
	addCmd.Flags().StringVarP(&addOutput, "output", "o", "", "Output file path (default: clipboard or stdout)")
	addCmd.Flags().StringVarP(&addLayer, "layer", "l", "", "Target segmentation layer name")
	addCmd.Flags().StringVar(&addColor, "color", "", "Segment color (e.g. #ff0000)")
	addCmd.Flags().BoolVar(&addUnpile, "unpile", false, "Reset to default template before adding selected neurons")
	addCmd.Flags().BoolVar(&addRootIDsOnly, "root-ids-only", false, "Just print root IDs, no state manipulation")
	addCmd.Flags().BoolVar(&addOpen, "open", false, "Open updated Neuroglancer URL in default browser")

	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	filters := &seatable.Filters{
		SuperClass:  addSuperClass,
		CellClass:   addCellClass,
		CellType:    addCellType,
		CellSubtype: addCellSubtype,
		Side:        addSide,
		Region:      addRegion,
		Tract:       addTract,
		Proofread:   addProofread,
	}

	if !filters.HasAny() {
		return fmt.Errorf("at least one filter flag is required (e.g. --cell-class, --super-class)")
	}
	if err := validateAddModeFlags(addUnpile, addState, addGenerate, addOutput, addRootIDsOnly); err != nil {
		return err
	}

	// Query SeaTable
	client, err := seatable.NewClient()
	if err != nil {
		return err
	}

	rows, err := seatable.QueryNeurons(client, filters)
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "No neurons found matching filters")
		return nil
	}

	rootIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.RootID != "" {
			rootIDs = append(rootIDs, r.RootID)
		}
	}

	fmt.Fprintf(os.Stderr, "Found %d neurons (%d with root IDs)\n", len(rows), len(rootIDs))

	// Root IDs only mode
	if addRootIDsOnly {
		fmt.Println(strings.Join(rootIDs, "\n"))
		return nil
	}

	// Load state
	result, err := resolveAddState(addState, addGenerate, addUnpile, addStateResolverDeps{
		loadState:     nglstate.LoadState,
		readClipboard: clipboard.Read,
		decodeURL:     nglstate.DecodeURL,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "State loaded from %s\n", result.Source)

	// Find segmentation layer and inject
	layer, _, err := nglstate.FindSegmentationLayer(result.State, addLayer)
	if err != nil {
		return err
	}

	nglstate.AddSegments(layer, rootIDs)
	nglstate.SetSegmentColor(layer, rootIDs, addColor)

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

func validateAddModeFlags(unpile bool, state string, generate bool, output string, rootIDsOnly bool) error {
	if !unpile {
		return nil
	}
	if state != "" {
		return fmt.Errorf("--unpile cannot be used with --state")
	}
	if generate {
		return fmt.Errorf("--unpile cannot be used with --generate")
	}
	if output != "" {
		return fmt.Errorf("--unpile cannot be used with --output")
	}
	if rootIDsOnly {
		return fmt.Errorf("--unpile cannot be used with --root-ids-only")
	}
	return nil
}

type addStateResolverDeps struct {
	loadState     func(stateArg string, generate bool) (*nglstate.LoadResult, error)
	readClipboard func() (string, error)
	decodeURL     func(url string) (map[string]interface{}, error)
}

func resolveAddState(stateArg string, generate bool, unpile bool, deps addStateResolverDeps) (*nglstate.LoadResult, error) {
	if unpile {
		return deps.loadState("", true)
	}
	if stateArg != "" {
		return deps.loadState(stateArg, false)
	}
	if generate {
		return deps.loadState("", true)
	}

	clip, err := deps.readClipboard()
	if err != nil {
		return nil, fmt.Errorf("reading clipboard: %w", err)
	}
	clip = strings.TrimSpace(clip)
	if !nglstate.IsNeuroglancerURL(clip) {
		return nil, fmt.Errorf("clipboard does not contain a valid Neuroglancer URL; use --unpile to start clean or --state to provide input")
	}

	state, err := deps.decodeURL(clip)
	if err != nil {
		return nil, fmt.Errorf("decoding Neuroglancer URL from clipboard: %w", err)
	}
	return &nglstate.LoadResult{
		State:       state,
		Source:      nglstate.SourceClipboard,
		OriginalURL: clip,
	}, nil
}
