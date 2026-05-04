package cmd

import (
	"fmt"
	"io"
	"math"
	"strings"

	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

var sideCheckCmd = &cobra.Command{
	Use:   "side-check",
	Short: "Check selected neuron sides against the nearest EPG/PEG neuron",
	Long: `Check selected neurons against the nearest valid EPG/PEG neuron by 3D
Euclidean distance. Prints one selected root_id per line when the selected
neuron has missing side data, missing or malformed position data, or the same
side as the nearest valid EPG/PEG neuron.`,
	Annotations: map[string]string{"requiresToken": "true"},
	Args:        cobra.NoArgs,
}

func init() {
	var (
		sideCheckCellType  string
		sideCheckCellClass string
	)

	sideCheckCmd.Flags().StringVar(&sideCheckCellType, "cell-type", "", "Check neurons with this cell_type")
	sideCheckCmd.Flags().StringVar(&sideCheckCellClass, "cell-class", "", "Check neurons with this cell_class")

	sideCheckCmd.RunE = func(cmd *cobra.Command, args []string) error {
		filters, err := validateSideCheckFilters(sideCheckCellType, sideCheckCellClass)
		if err != nil {
			return err
		}

		client, err := seatable.NewClient()
		if err != nil {
			return err
		}

		meta, err := client.FetchMetadata()
		if err != nil {
			return fmt.Errorf("fetching column metadata: %w", err)
		}
		regionOpts := seatable.SelectOptionMap(meta, "region")

		targets, err := seatable.QueryNeuronPositions(client, filters, regionOpts)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), "No neurons found matching criteria")
			return nil
		}

		candidates, err := seatable.QueryNeuronsWithPosition(client, regionOpts)
		if err != nil {
			return err
		}
		references := validSideReferenceRows(candidates)
		if len(references) == 0 {
			return fmt.Errorf("no valid EPG/PEG neurons found with position coordinates and side")
		}

		report := buildSideCheckReport(targets, references)
		if err := writeSideCheckProblems(cmd.OutOrStdout(), report); err != nil {
			return err
		}
		writeSideCheckSummary(cmd.ErrOrStderr(), report)
		return nil
	}

	rootCmd.AddCommand(sideCheckCmd)
}

type sideCheckReport struct {
	Checked             int
	ProblemRootIDs      []string
	MissingRootIDCount  int
	UnprintedProblemCnt int
}

func validateSideCheckFilters(cellType, cellClass string) (*seatable.Filters, error) {
	cellType = strings.TrimSpace(cellType)
	cellClass = strings.TrimSpace(cellClass)

	if cellType == "" && cellClass == "" {
		return nil, fmt.Errorf("provide exactly one of --cell-type or --cell-class")
	}
	if cellType != "" && cellClass != "" {
		return nil, fmt.Errorf("cannot combine --cell-type and --cell-class")
	}
	if cellType != "" {
		return &seatable.Filters{CellType: cellType}, nil
	}
	return &seatable.Filters{CellClass: cellClass}, nil
}

func buildSideCheckReport(targets, references []seatable.NeuronPositionRow) sideCheckReport {
	report := sideCheckReport{Checked: len(targets)}

	for _, target := range targets {
		if strings.TrimSpace(target.RootID) == "" {
			report.MissingRootIDCount++
		}

		if !sideCheckHasProblem(target, references) {
			continue
		}

		if strings.TrimSpace(target.RootID) == "" {
			report.UnprintedProblemCnt++
			continue
		}
		report.ProblemRootIDs = append(report.ProblemRootIDs, target.RootID)
	}

	return report
}

func sideCheckHasProblem(target seatable.NeuronPositionRow, references []seatable.NeuronPositionRow) bool {
	if normalizeSide(target.Side) == "" {
		return true
	}
	if !target.HasPosition() {
		return true
	}

	nearest := nearestSideReference(target, references)
	if nearest == nil {
		return true
	}
	return normalizeSide(target.Side) == normalizeSide(nearest.Side)
}

func nearestSideReference(target seatable.NeuronPositionRow, references []seatable.NeuronPositionRow) *seatable.NeuronPositionRow {
	var closest *seatable.NeuronPositionRow
	closestDist := math.MaxFloat64

	for i := range references {
		candidate := &references[i]
		if !isValidSideReference(*candidate) {
			continue
		}
		d := euclideanDistance(target.X, target.Y, target.Z, candidate.X, candidate.Y, candidate.Z)
		if d < closestDist {
			closestDist = d
			closest = candidate
		}
	}

	return closest
}

func validSideReferenceRows(rows []seatable.NeuronPositionRow) []seatable.NeuronPositionRow {
	valid := make([]seatable.NeuronPositionRow, 0, len(rows))
	for _, row := range rows {
		if isValidSideReference(row) {
			valid = append(valid, row)
		}
	}
	return valid
}

func isValidSideReference(row seatable.NeuronPositionRow) bool {
	return row.HasPosition() && normalizeSide(row.Side) != ""
}

func normalizeSide(side string) string {
	return strings.ToLower(strings.TrimSpace(side))
}

func writeSideCheckProblems(w io.Writer, report sideCheckReport) error {
	for _, rootID := range report.ProblemRootIDs {
		if _, err := fmt.Fprintln(w, rootID); err != nil {
			return err
		}
	}
	return nil
}

func writeSideCheckSummary(w io.Writer, report sideCheckReport) {
	fmt.Fprintf(w, "Checked %d neurons; printed %d problem root IDs\n", report.Checked, len(report.ProblemRootIDs))
	if report.MissingRootIDCount > 0 {
		fmt.Fprintf(w, "%d rows had no root_id\n", report.MissingRootIDCount)
	}
	if report.UnprintedProblemCnt > 0 {
		fmt.Fprintf(w, "%d problem rows could not be printed because root_id was missing\n", report.UnprintedProblemCnt)
	}
}
