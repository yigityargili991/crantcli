package cmd

import (
	"reflect"
	"testing"

	"crantcli/internal/seatable"
)

func TestValidateSideCheckFilters(t *testing.T) {
	tests := []struct {
		name      string
		cellType  string
		cellClass string
		want      seatable.Filters
		wantErr   bool
	}{
		{
			name:     "cell type",
			cellType: "PFN",
			want:     seatable.Filters{CellType: "PFN"},
		},
		{
			name:      "cell class",
			cellClass: "cx",
			want:      seatable.Filters{CellClass: "cx"},
		},
		{
			name:      "trims input",
			cellClass: " cx ",
			want:      seatable.Filters{CellClass: "cx"},
		},
		{
			name:    "missing filter",
			wantErr: true,
		},
		{
			name:      "conflicting filters",
			cellType:  "PFN",
			cellClass: "cx",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateSideCheckFilters(tt.cellType, tt.cellClass)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSideCheckFilters returned error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got == nil {
				t.Fatal("validateSideCheckFilters returned nil filters")
			}
			if *got != tt.want {
				t.Fatalf("filters = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestBuildSideCheckReport(t *testing.T) {
	references := []seatable.NeuronPositionRow{
		positionRow("epg-left", "left", 0, 0, 0, true),
		positionRow("epg-right", "right", 100, 0, 0, true),
	}

	tests := []struct {
		name    string
		targets []seatable.NeuronPositionRow
		wantIDs []string
	}{
		{
			name: "matching side is clean",
			targets: []seatable.NeuronPositionRow{
				positionRow("target-ok", " LEFT ", 1, 0, 0, true),
			},
		},
		{
			name: "mismatched side is printed",
			targets: []seatable.NeuronPositionRow{
				positionRow("target-mismatch", "right", 1, 0, 0, true),
			},
			wantIDs: []string{"target-mismatch"},
		},
		{
			name: "missing side is printed",
			targets: []seatable.NeuronPositionRow{
				positionRow("target-no-side", "", 1, 0, 0, true),
			},
			wantIDs: []string{"target-no-side"},
		},
		{
			name: "missing position is printed",
			targets: []seatable.NeuronPositionRow{
				positionRow("target-no-position", "left", 0, 0, 0, false),
			},
			wantIDs: []string{"target-no-position"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := buildSideCheckReport(tt.targets, references)
			if !reflect.DeepEqual(report.ProblemRootIDs, tt.wantIDs) {
				t.Fatalf("ProblemRootIDs = %v, want %v", report.ProblemRootIDs, tt.wantIDs)
			}
		})
	}
}

func TestBuildSideCheckReportCountsMissingRootID(t *testing.T) {
	references := []seatable.NeuronPositionRow{
		positionRow("epg-left", "left", 0, 0, 0, true),
	}
	targets := []seatable.NeuronPositionRow{
		positionRow("", "right", 1, 0, 0, true),
	}

	report := buildSideCheckReport(targets, references)
	if len(report.ProblemRootIDs) != 0 {
		t.Fatalf("ProblemRootIDs = %v, want none for missing root_id", report.ProblemRootIDs)
	}
	if report.MissingRootIDCount != 1 {
		t.Fatalf("MissingRootIDCount = %d, want 1", report.MissingRootIDCount)
	}
	if report.UnprintedProblemCnt != 1 {
		t.Fatalf("UnprintedProblemCnt = %d, want 1", report.UnprintedProblemCnt)
	}
}

func TestInvalidEPGReferencesAreExcluded(t *testing.T) {
	references := []seatable.NeuronPositionRow{
		positionRow("invalid-no-position", "left", 0, 0, 0, false),
		positionRow("invalid-no-side", "", 0, 0, 0, true),
		positionRow("valid-right", "right", 100, 0, 0, true),
	}
	targets := []seatable.NeuronPositionRow{
		positionRow("target", "right", 0, 0, 0, true),
	}

	report := buildSideCheckReport(targets, references)
	if len(report.ProblemRootIDs) != 0 {
		t.Fatalf("ProblemRootIDs = %v, want invalid references excluded", report.ProblemRootIDs)
	}

	valid := validSideReferenceRows(references)
	if got, want := len(valid), 1; got != want {
		t.Fatalf("valid references = %d, want %d", got, want)
	}
	if valid[0].RootID != "valid-right" {
		t.Fatalf("valid[0].RootID = %q, want valid-right", valid[0].RootID)
	}
}

func positionRow(rootID, side string, x, y, z float64, hasPosition bool) seatable.NeuronPositionRow {
	return seatable.NeuronPositionRow{
		RootID:      rootID,
		Side:        side,
		X:           x,
		Y:           y,
		Z:           z,
		PositionSet: hasPosition,
	}
}
