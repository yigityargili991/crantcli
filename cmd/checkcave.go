package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"crantcli/internal/cave"
	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"
	"crantcli/internal/textout"

	"github.com/spf13/cobra"
)

var errStaleFound = errors.New("stale root IDs found")

const (
	statusOK           = "ok"
	statusStale        = "STALE"
	statusNoSupervoxel = "no_supervoxel"
	statusError        = "error"
)

var (
	checkCaveNewSeaTableClient = seatable.NewClient
	checkCaveNewCAVEClient     = cave.NewClient
	checkCaveQueryNeurons      = seatable.QueryNeuronsForCaveCheck
)

// checkCaveFilterFlags lists the classification filters check-cave accepts.
var checkCaveFilterFlags = []string{
	"super-class",
	"cell-class",
	"cell-type",
	"cell-subtype",
	"side",
	"region",
	"tract",
	"nerve",
	"hemilineage",
	"proofread",
}

var checkCaveCmd = &cobra.Command{
	Use:   "check-cave [root_id...]",
	Short: "Check if root IDs are still current in CAVE",
	Long: `Check whether root IDs stored in SeaTable still match the current CAVE
chunkedgraph by looking up each neuron's supervoxel_id.

Supervoxel IDs are stable, but root IDs can change when proofreading edits
(merges/splits) happen in CAVE. This command detects stale root IDs.

The exit status is 0 on success, 1 when --quiet finds stale root IDs, and 2
when the command fails or any root ID cannot be checked.`,
	Example: `  # Check a single root ID
  crantcli check-cave 720575940610453042

  # Check multiple root IDs
  crantcli check-cave 720575940610453042 720575940631928371

  # Check all neurons in the table
  crantcli check-cave --all

  # Check only ER neurons
  crantcli check-cave --all --cell-type ER

  # Only print stale entries (exit code 1 if any found)
  crantcli check-cave --all --quiet

  # Print stale root mappings as old_root_id<TAB>current_cave_root_id
  crantcli check-cave --all --mapping

  # Refresh stale segment IDs in a Neuroglancer state
  crantcli check-cave --all --refresh-state -s state.json -o refreshed.json`,
	Annotations: map[string]string{"requiresToken": "true"},
}

const batchThreshold = 10

func init() {
	var (
		checkAll          bool
		checkQuiet        bool
		checkSuperClass   string
		checkCellClass    string
		checkCellType     string
		checkCellSubtype  string
		checkSide         string
		checkRegion       string
		checkTract        string
		checkNerve        string
		checkHemilineage  string
		checkProofread    string
		checkMapping      bool
		checkRefreshState bool
		checkState        string
		checkOutput       string
		checkLayer        string
	)

	checkCaveCmd.Flags().BoolVar(&checkAll, "all", false, "Check all neurons (or filtered subset)")
	checkCaveCmd.Flags().BoolVarP(&checkQuiet, "quiet", "q", false, "Suppress progress/summary; only output stale entries; exit code 1 if any found")
	checkCaveCmd.Flags().StringVar(&checkSuperClass, "super-class", "", "Filter by super_class")
	checkCaveCmd.Flags().StringVar(&checkCellClass, "cell-class", "", "Filter by cell_class")
	checkCaveCmd.Flags().StringVar(&checkCellType, "cell-type", "", "Filter by cell_type")
	checkCaveCmd.Flags().StringVar(&checkCellSubtype, "cell-subtype", "", "Filter by cell_subtype")
	checkCaveCmd.Flags().StringVar(&checkSide, "side", "", "Filter by side")
	checkCaveCmd.Flags().StringVar(&checkRegion, "region", "", "Filter by region")
	checkCaveCmd.Flags().StringVar(&checkTract, "tract", "", "Filter by tract")
	checkCaveCmd.Flags().StringVar(&checkNerve, "nerve", "", "Filter by nerve")
	checkCaveCmd.Flags().StringVar(&checkHemilineage, "hemilineage", "", "Filter by hemilineage")
	checkCaveCmd.Flags().StringVar(&checkProofread, "proofread", "", "Filter by proofread status")
	checkCaveCmd.Flags().BoolVar(&checkMapping, "mapping", false, "Print stale mappings as old_root_id<TAB>current_cave_root_id")
	checkCaveCmd.Flags().BoolVar(&checkRefreshState, "refresh-state", false, "Replace stale segment IDs in a Neuroglancer state with current CAVE root IDs")
	checkCaveCmd.Flags().StringVarP(&checkState, "state", "s", "", "Neuroglancer state (URL or file path) for --refresh-state (default: piped stdin, then a clipboard URL)")
	checkCaveCmd.Flags().StringVarP(&checkOutput, "output", "o", "", "Output file path for --refresh-state (default: clipboard or stdout)")
	checkCaveCmd.Flags().StringVarP(&checkLayer, "layer", "l", "", "Target segmentation layer name for --refresh-state")
	checkCaveCmd.ValidArgsFunction = noFileCompletion
	registerClassificationFlagCompletions(checkCaveCmd, checkCaveFilterFlags...)
	mustRegisterFlagCompletion(checkCaveCmd, "layer", noFileCompletion)

	checkCaveCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateCheckCaveFilters(cmd); err != nil {
			return err
		}
		filters := &seatable.Filters{
			SuperClass:  checkSuperClass,
			CellClass:   checkCellClass,
			CellType:    checkCellType,
			CellSubtype: checkCellSubtype,
			Side:        checkSide,
			Region:      checkRegion,
			Tract:       checkTract,
			Nerve:       checkNerve,
			Hemilineage: checkHemilineage,
			Proofread:   checkProofread,
		}

		hasFilters := filters.HasAny()
		hasArgs := len(args) > 0

		if hasArgs && (checkAll || hasFilters) {
			return fmt.Errorf("cannot combine root_id arguments with --all or filter flags")
		}
		if !hasArgs && !checkAll && !hasFilters {
			return fmt.Errorf("provide root_id arguments, use --all, or specify filter flags")
		}
		if !checkRefreshState && (checkState != "" || checkOutput != "" || checkLayer != "") {
			return fmt.Errorf("--state, --output, and --layer require --refresh-state")
		}
		if checkRefreshState && checkMapping && checkOutput == "" {
			return fmt.Errorf("combine --mapping and --refresh-state only with --output to avoid mixing mapping and state JSON on stdout")
		}

		rootIDs, err := canonicalRootIDs(args)
		if err != nil {
			return err
		}

		// Resolve the state and its layer before the slow lookups, so a bad
		// --state or --layer fails at once and the clipboard is read now
		// rather than whenever the check ends.
		var state *nglstate.LoadResult
		var layer map[string]interface{}
		if checkRefreshState {
			state, err = nglstate.LoadInputState(checkState)
			if err != nil {
				return err
			}
			layer, _, err = nglstate.FindSegmentationLayer(state.State, checkLayer)
			if err != nil {
				return err
			}
		}

		stClient, err := checkCaveNewSeaTableClient()
		if err != nil {
			return err
		}

		caveClient, err := checkCaveNewCAVEClient()
		if err != nil {
			return err
		}

		var neurons []seatable.NeuronCaveCheckRow
		if hasArgs {
			neurons, err = queryRootSupervoxels(stClient, rootIDs)
		} else {
			neurons, err = checkCaveQueryNeurons(stClient, filters)
		}
		if err != nil {
			return err
		}

		if len(neurons) == 0 {
			if !checkQuiet {
				fmt.Fprintln(os.Stderr, "No neurons found matching criteria")
			}
			// A refresh still delivers the state, unchanged, so a script gets
			// the output it asked for.
			if !checkRefreshState {
				return nil
			}
		} else if !checkQuiet && !checkMapping {
			fmt.Fprintf(os.Stderr, "Checking %d neurons against CAVE...\n", len(neurons))
		}

		results, err := checkNeurons(caveClient, neurons)
		if err != nil {
			return err
		}

		if checkMapping || checkRefreshState {
			if err := failOnResultErrors(os.Stderr, results); err != nil {
				return err
			}
		}

		staleMappings := staleRootMappings(results)
		var staleCount int

		if checkMapping {
			if err := writeMappings(os.Stdout, staleMappings); err != nil {
				return err
			}
			staleCount = len(staleMappings)
		} else if checkRefreshState {
			staleCount = len(staleMappings)
		} else {
			var errCount int
			staleCount, errCount, err = printResults(results, checkQuiet)
			if err != nil {
				return err
			}
			// A root that could not be checked must not pass for a current one.
			if errCount > 0 {
				return incompleteCheckError(errCount, len(results))
			}
		}

		if checkRefreshState {
			if err := refreshStateSegments(state, layer, staleMappings, checkOutput); err != nil {
				return err
			}
		}

		if checkQuiet && staleCount > 0 {
			return errStaleFound
		}
		return nil
	}

	rootCmd.AddCommand(checkCaveCmd)
}

// validateCheckCaveFilters rejects a blank filter value. It names no subset:
// the query drops an empty one, which widens --all to every neuron.
func validateCheckCaveFilters(cmd *cobra.Command) error {
	for _, name := range checkCaveFilterFlags {
		if !cmd.Flags().Changed(name) {
			continue
		}
		if value, _ := cmd.Flags().GetString(name); strings.TrimSpace(value) == "" {
			return fmt.Errorf("--%s cannot be empty", name)
		}
	}
	return nil
}

// parseRootID parses a stored or typed root ID, tolerating surrounding
// whitespace and leading zeros.
func parseRootID(s string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(s), 10, 64)
}

// canonicalRootIDs validates root_id arguments and returns them in canonical
// form, in argument order, without duplicates.
func canonicalRootIDs(args []string) ([]string, error) {
	ids := make([]string, 0, len(args))
	seen := make(map[string]bool, len(args))
	for _, arg := range args {
		rootID, err := parseRootID(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid root_id %q: %w", arg, err)
		}
		id := strconv.FormatUint(rootID, 10)
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// queryRootSupervoxels returns every row stored under the given root IDs, in
// argument order. One root has several rows when it held several neurons, and
// after a split each row's supervoxel can resolve to a different root.
func queryRootSupervoxels(client *seatable.Client, rootIDs []string) ([]seatable.NeuronCaveCheckRow, error) {
	rows, err := checkCaveQueryNeurons(client, &seatable.Filters{RootIDs: rootIDs})
	if err != nil {
		return nil, err
	}
	byRoot := make(map[string][]seatable.NeuronCaveCheckRow, len(rootIDs))
	for _, row := range rows {
		key := row.RootID
		if rootID, err := parseRootID(row.RootID); err == nil {
			key = strconv.FormatUint(rootID, 10)
		}
		byRoot[key] = append(byRoot[key], row)
	}
	neurons := make([]seatable.NeuronCaveCheckRow, 0, len(rows))
	for _, id := range rootIDs {
		if len(byRoot[id]) == 0 {
			return nil, fmt.Errorf("no neuron found with root_id %q", id)
		}
		neurons = append(neurons, byRoot[id]...)
	}
	return neurons, nil
}

type checkResult struct {
	RootID       string
	SupervoxelID string
	CaveRootID   string
	Status       string
	Err          error
}

func checkNeurons(caveClient *cave.Client, neurons []seatable.NeuronCaveCheckRow) ([]checkResult, error) {
	type indexed struct {
		idx    int
		svID   uint64
		rootID uint64
	}
	results := make([]checkResult, len(neurons))
	var withSV []indexed

	for i, n := range neurons {
		results[i] = checkResult{
			RootID:       n.RootID,
			SupervoxelID: strings.TrimSpace(n.SupervoxelID),
			CaveRootID:   "-",
		}
		// Parse as uint64 to compare numerically (avoids leading-zero/whitespace
		// mismatches), and report the canonical form, which is how states and
		// --mapping spell the ID.
		rootID, rootErr := parseRootID(n.RootID)
		if rootErr == nil {
			results[i].RootID = strconv.FormatUint(rootID, 10)
		}
		if results[i].SupervoxelID == "" {
			results[i].Status = statusNoSupervoxel
			continue
		}
		svID, err := strconv.ParseUint(results[i].SupervoxelID, 10, 64)
		if err != nil {
			results[i].Status = statusError
			results[i].Err = fmt.Errorf("invalid supervoxel_id %q: %w", n.SupervoxelID, err)
			continue
		}
		if rootErr != nil {
			results[i].Status = statusError
			results[i].Err = fmt.Errorf("invalid root_id %q: %w", n.RootID, rootErr)
			continue
		}
		withSV = append(withSV, indexed{idx: i, svID: svID, rootID: rootID})
	}

	if len(withSV) == 0 {
		return results, nil
	}

	setResult := func(item indexed, caveRoot uint64) {
		// CAVE's batch endpoint answers 0 for a supervoxel it cannot resolve.
		// No root is 0 or equal to one of its own supervoxels, so either means
		// the lookup failed, not that the root changed.
		if caveRoot == 0 || caveRoot == item.svID {
			results[item.idx].Status = statusError
			results[item.idx].Err = fmt.Errorf("CAVE has no root for supervoxel %d", item.svID)
			return
		}
		results[item.idx].CaveRootID = strconv.FormatUint(caveRoot, 10)
		if item.rootID == caveRoot {
			results[item.idx].Status = statusOK
		} else {
			results[item.idx].Status = statusStale
		}
	}

	if len(withSV) <= batchThreshold {
		for _, item := range withSV {
			caveRoot, err := caveClient.GetRootID(item.svID)
			if err != nil {
				results[item.idx].Status = statusError
				results[item.idx].Err = err
				continue
			}
			setResult(item, caveRoot)
		}
	} else {
		svIDs := make([]uint64, len(withSV))
		for i, item := range withSV {
			svIDs[i] = item.svID
		}
		caveRoots, err := caveClient.GetRootIDs(svIDs)
		if err != nil {
			return nil, fmt.Errorf("CAVE batch lookup: %w", err)
		}
		if len(caveRoots) != len(svIDs) {
			return nil, fmt.Errorf("CAVE returned %d root IDs for %d supervoxels", len(caveRoots), len(svIDs))
		}
		for i, item := range withSV {
			setResult(item, caveRoots[i])
		}
	}

	return results, nil
}

// printResults writes the result table and summary and returns the number of
// stale and failed checks.
func printResults(results []checkResult, quiet bool) (int, int, error) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	var staleCount, okCount, noSVCount, errCount int
	headerPrinted := false

	for _, r := range results {
		switch r.Status {
		case statusOK:
			okCount++
		case statusStale:
			staleCount++
		case statusNoSupervoxel:
			noSVCount++
		case statusError:
			errCount++
		}

		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  error for %s: %v\n", r.RootID, r.Err)
		}
		if quiet && r.Status != statusStale {
			continue
		}
		if !headerPrinted {
			fmt.Fprintln(w, "root_id\tsupervoxel_id\tcave_root_id\tstatus")
			headerPrinted = true
		}

		svDisplay := r.SupervoxelID
		if svDisplay == "" {
			svDisplay = "(none)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", textout.Sanitize(r.RootID), textout.Sanitize(svDisplay), textout.Sanitize(r.CaveRootID), r.Status)
	}

	if err := w.Flush(); err != nil {
		return 0, 0, fmt.Errorf("writing output: %w", err)
	}

	if !quiet {
		parts := []string{fmt.Sprintf("%d checked", len(results))}
		if okCount > 0 {
			parts = append(parts, fmt.Sprintf("%d ok", okCount))
		}
		if staleCount > 0 {
			parts = append(parts, fmt.Sprintf("%d stale", staleCount))
		}
		if noSVCount > 0 {
			parts = append(parts, fmt.Sprintf("%d no supervoxel", noSVCount))
		}
		if errCount > 0 {
			parts = append(parts, fmt.Sprintf("%d errors", errCount))
		}
		fmt.Fprintf(os.Stderr, "%s\n", strings.Join(parts, ", "))
	}

	return staleCount, errCount, nil
}

func failOnResultErrors(w io.Writer, results []checkResult) error {
	errCount, err := writeResultErrors(w, results)
	if err != nil {
		return err
	}
	if errCount > 0 {
		return incompleteCheckError(errCount, len(results))
	}
	return nil
}

func incompleteCheckError(failed, total int) error {
	return fmt.Errorf("%d of %d checks failed; output incomplete", failed, total)
}

func writeResultErrors(w io.Writer, results []checkResult) (int, error) {
	var errCount int
	for _, r := range results {
		if r.Err == nil {
			continue
		}
		errCount++
		if _, err := fmt.Fprintf(w, "  error for %s: %v\n", r.RootID, r.Err); err != nil {
			return errCount, fmt.Errorf("writing error output: %w", err)
		}
	}
	return errCount, nil
}

// staleRootMappings lists each distinct old-to-current pair once. A root split
// into several neurons yields one pair per current root.
func staleRootMappings(results []checkResult) []rootMapping {
	mappings := make([]rootMapping, 0)
	seen := make(map[rootMapping]bool)
	for _, r := range results {
		if r.Status != statusStale || r.CaveRootID == "" || r.CaveRootID == "-" {
			continue
		}
		mapping := rootMapping{OldRootID: r.RootID, CurrentRootID: r.CaveRootID}
		if !seen[mapping] {
			seen[mapping] = true
			mappings = append(mappings, mapping)
		}
	}
	return mappings
}

type rootMapping struct {
	OldRootID     string
	CurrentRootID string
}

func writeMappings(w io.Writer, mappings []rootMapping) error {
	for _, mapping := range mappings {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", mapping.OldRootID, mapping.CurrentRootID); err != nil {
			return fmt.Errorf("writing mapping output: %w", err)
		}
	}
	return nil
}

func refreshStateSegments(result *nglstate.LoadResult, layer map[string]interface{}, mappings []rootMapping, outputFile string) error {
	replaced := nglstate.ReplaceSegments(layer, rootMappingMap(mappings))
	fmt.Fprintf(os.Stderr, "Replaced %d stale segments in state\n", replaced)

	return nglstate.WriteState(result, outputFile)
}

// rootMappingMap groups current roots by old root; a split root has several.
func rootMappingMap(mappings []rootMapping) map[string][]string {
	result := make(map[string][]string, len(mappings))
	for _, mapping := range mappings {
		result[mapping.OldRootID] = append(result[mapping.OldRootID], mapping.CurrentRootID)
	}
	return result
}
