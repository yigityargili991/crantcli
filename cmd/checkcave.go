package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"crantinject/internal/cave"
	"crantinject/internal/seatable"

	"github.com/spf13/cobra"
)

// errStaleFound is returned when --quiet mode detects stale root IDs.
// root.go handles this by exiting with code 1 without printing the error.
var errStaleFound = errors.New("stale root IDs found")

var checkCaveCmd = &cobra.Command{
	Use:   "check-cave [root_id...]",
	Short: "Check if root IDs are still current in CAVE",
	Long: `Check whether root IDs stored in SeaTable still match the current CAVE
chunkedgraph by looking up each neuron's supervoxel_id.

Supervoxel IDs are stable, but root IDs can change when proofreading edits
(merges/splits) happen in CAVE. This command detects stale root IDs.

Examples:
  # Check a single root ID
  crantinject check-cave 720575940610453042

  # Check multiple root IDs
  crantinject check-cave 720575940610453042 720575940631928371

  # Check all neurons in the table
  crantinject check-cave --all

  # Check only kenyon cells
  crantinject check-cave --all --cell-class kenyon_cell

  # Only print stale entries (exit code 1 if any found)
  crantinject check-cave --all --quiet`,
	Annotations: map[string]string{"requiresToken": "true"},
}

const batchThreshold = 10

func init() {
	var (
		checkAll        bool
		checkQuiet      bool
		checkSuperClass string
		checkCellClass  string
		checkCellType   string
		checkCellSubtype string
		checkSide       string
		checkRegion     string
		checkTract      string
		checkNerve      string
		checkHemilineage string
		checkProofread  string
	)

	checkCaveCmd.Flags().BoolVar(&checkAll, "all", false, "Check all neurons (or filtered subset)")
	checkCaveCmd.Flags().BoolVarP(&checkQuiet, "quiet", "q", false, "Only print stale entries; exit code 1 if any found")
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

	checkCaveCmd.RunE = func(cmd *cobra.Command, args []string) error {
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

		stClient, err := seatable.NewClient()
		if err != nil {
			return err
		}

		caveClient, err := cave.NewClient()
		if err != nil {
			return err
		}

		var neurons []seatable.NeuronCaveCheckRow

		if hasArgs {
			for _, rootID := range args {
				row, err := seatable.QueryNeuronSupervoxel(stClient, rootID)
				if err != nil {
					return fmt.Errorf("querying root_id %s: %w", rootID, err)
				}
				if row == nil {
					return fmt.Errorf("no neuron found with root_id %q", rootID)
				}
				neurons = append(neurons, *row)
			}
		} else {
			neurons, err = seatable.QueryNeuronsForCaveCheck(stClient, filters)
			if err != nil {
				return err
			}
		}

		if len(neurons) == 0 {
			fmt.Fprintln(os.Stderr, "No neurons found matching criteria")
			return nil
		}

		fmt.Fprintf(os.Stderr, "Checking %d neurons against CAVE...\n", len(neurons))

		results, err := checkNeurons(caveClient, neurons)
		if err != nil {
			return err
		}

		staleCount := printResults(results, checkQuiet)

		if checkQuiet && staleCount > 0 {
			return errStaleFound
		}
		return nil
	}

	rootCmd.AddCommand(checkCaveCmd)
}

type checkResult struct {
	RootID       string
	SupervoxelID string
	CaveRootID   string
	Status       string // "ok", "STALE", "no_supervoxel", "error"
	Err          error
}

func checkNeurons(caveClient *cave.Client, neurons []seatable.NeuronCaveCheckRow) ([]checkResult, error) {
	// Separate neurons with and without supervoxel IDs.
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
			SupervoxelID: n.SupervoxelID,
		}
		if n.SupervoxelID == "" {
			results[i].CaveRootID = "-"
			results[i].Status = "no_supervoxel"
			continue
		}
		svID, err := strconv.ParseUint(n.SupervoxelID, 10, 64)
		if err != nil {
			results[i].CaveRootID = "-"
			results[i].Status = "error"
			results[i].Err = fmt.Errorf("invalid supervoxel_id %q: %w", n.SupervoxelID, err)
			continue
		}
		// Parse root_id as uint64 for numeric comparison to avoid
		// string formatting differences (leading zeros, whitespace).
		rootID, err := strconv.ParseUint(strings.TrimSpace(n.RootID), 10, 64)
		if err != nil {
			results[i].CaveRootID = "-"
			results[i].Status = "error"
			results[i].Err = fmt.Errorf("invalid root_id %q: %w", n.RootID, err)
			continue
		}
		withSV = append(withSV, indexed{idx: i, svID: svID, rootID: rootID})
	}

	if len(withSV) == 0 {
		return results, nil
	}

	if len(withSV) <= batchThreshold {
		// Individual lookups.
		for _, item := range withSV {
			caveRoot, err := caveClient.GetRootID(item.svID)
			if err != nil {
				results[item.idx].CaveRootID = "-"
				results[item.idx].Status = "error"
				results[item.idx].Err = err
				continue
			}
			results[item.idx].CaveRootID = strconv.FormatUint(caveRoot, 10)
			if item.rootID == caveRoot {
				results[item.idx].Status = "ok"
			} else {
				results[item.idx].Status = "STALE"
			}
		}
	} else {
		// Batch lookup.
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
			results[item.idx].CaveRootID = strconv.FormatUint(caveRoots[i], 10)
			if item.rootID == caveRoots[i] {
				results[item.idx].Status = "ok"
			} else {
				results[item.idx].Status = "STALE"
			}
		}
	}

	return results, nil
}

func printResults(results []checkResult, quiet bool) int {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	staleCount := 0
	headerPrinted := false

	for _, r := range results {
		if r.Status == "STALE" {
			staleCount++
		}
		// Always print errors to stderr, even in quiet mode.
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  error for %s: %v\n", r.RootID, r.Err)
		}
		if quiet && r.Status != "STALE" {
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

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.RootID, svDisplay, r.CaveRootID, r.Status)
	}

	w.Flush()

	okCount := 0
	noSVCount := 0
	errCount := 0
	for _, r := range results {
		switch r.Status {
		case "ok":
			okCount++
		case "no_supervoxel":
			noSVCount++
		case "error":
			errCount++
		}
	}

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

	return staleCount
}
