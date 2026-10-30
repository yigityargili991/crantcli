package cmd

import (
	"strings"
	"testing"

	"crantcli/internal/seatable"
)

func TestFormatLookupColumnOutput(t *testing.T) {
	got := formatLookupColumnOutput("CX, LW", "720575940610453042")
	want := "CX, LW\t720575940610453042"
	if got != want {
		t.Fatalf("formatLookupColumnOutput() = %q, want %q", got, want)
	}

	// Region and root ID are display strings; a control character must not
	// break the one-line, tab-separated shape.
	if got := formatLookupColumnOutput("CX\nLW", "1\t2"); strings.Count(got, "\n")+strings.Count(got, "\t") != 1 {
		t.Fatalf("formatLookupColumnOutput() = %q, want one tab and no newlines", got)
	}
}

func TestLookupColumnDeprecatedAndHidden(t *testing.T) {
	if !lookupColumnCmd.Hidden {
		t.Error("lookup-column must be hidden from --help")
	}
	if lookupColumnCmd.Deprecated == "" {
		t.Fatal("lookup-column must be deprecated")
	}
	for _, want := range []string{"root-info", "v0.25.0"} {
		if !strings.Contains(lookupColumnCmd.Deprecated, want) {
			t.Errorf("lookup-column deprecation warning %q lacks %q", lookupColumnCmd.Deprecated, want)
		}
	}
}

// stubLookupColumnDataSource replaces the root-info data source factory and the
// token lookup for one lookup-column run, so the command reaches no SeaTable.
// It returns the number of times the factory ran.
func stubLookupColumnDataSource(t *testing.T, data rootInfoDataSource) *int {
	t.Helper()
	calls := 0
	previousDataSource := rootInfoNewDataSource
	previousGetAPIToken := getAPIToken
	rootInfoNewDataSource = func() (rootInfoDataSource, error) {
		calls++
		return data, nil
	}
	getAPIToken = func() string { return "test-token" }
	t.Cleanup(func() {
		rootInfoNewDataSource = previousDataSource
		getAPIToken = previousGetAPIToken
	})
	return &calls
}

// runLookupColumnForTest runs the registered command with clean flags.
func runLookupColumnForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	resetCommandFlags(lookupColumnCmd)
	t.Cleanup(func() { resetCommandFlags(lookupColumnCmd) })
	return executeRootForTest(t, append([]string{"lookup-column"}, args...)...)
}

// TestLookupColumnPositionOutput pins the deprecated command's output: the
// nearest column's region and root ID, still only those two fields.
func TestLookupColumnPositionOutput(t *testing.T) {
	stubLookupColumnDataSource(t, positionTestDataSource())

	out, _, err := runLookupColumnForTest(t, "--pos", "100.5,200.3,300.1")
	if err != nil {
		t.Fatalf("lookup-column --pos error = %v", err)
	}
	// Cobra prints the deprecation notice first; the command's own output is
	// the last line.
	if !strings.HasSuffix(out, "CX\t222\n") {
		t.Fatalf("lookup-column --pos output = %q, want it to end with the region and root ID", out)
	}
	for _, unwanted := range []string{"distance", "side", "nearest_column"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("lookup-column --pos output = %q, must not print %q", out, unwanted)
		}
	}
}

func TestLookupColumnArgumentsMatchRootInfo(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "neither root ID nor pos", wantErr: "provide a root_id argument or use --pos x,y,z"},
		{
			name:    "both root ID and pos",
			args:    []string{"111", "--pos", "1,2,3"},
			wantErr: "cannot combine root_id argument with --pos flag",
		},
		{
			name:    "both root ID and empty pos",
			args:    []string{"111", "--pos", ""},
			wantErr: "cannot combine root_id argument with --pos flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := stubLookupColumnDataSource(t, positionTestDataSource())
			out, _, err := runLookupColumnForTest(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("lookup-column %v error = %v, want %q", tt.args, err, tt.wantErr)
			}
			if *calls != 0 {
				t.Fatalf("lookup-column %v created %d data sources, want none", tt.args, *calls)
			}
			if strings.Contains(out, "\t") {
				t.Fatalf("lookup-column %v wrote output: %q", tt.args, out)
			}
		})
	}
}

// root-info rejects a changed-but-empty --pos instead of falling through to the
// root-ID path; the deprecated command must fail the same way.
func TestLookupColumnRejectsEmptyPos(t *testing.T) {
	calls := stubLookupColumnDataSource(t, positionTestDataSource())
	for _, args := range [][]string{{"--pos", ""}, {"--pos="}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, _, err := runLookupColumnForTest(t, args...)
			if err == nil || !strings.Contains(err.Error(), "--pos requires a position as x,y,z") {
				t.Fatalf("lookup-column %v error = %v, want clear --pos error", args, err)
			}
			if strings.Contains(out, "\t") {
				t.Fatalf("lookup-column %v wrote output: %q", args, out)
			}
		})
	}
	if *calls != 0 {
		t.Fatalf("empty --pos created %d data sources, want none", *calls)
	}
}

func TestLookupColumnPositionInvalidFailsBeforeDataSource(t *testing.T) {
	calls := stubLookupColumnDataSource(t, positionTestDataSource())
	_, _, err := runLookupColumnForTest(t, "--pos", "1,2")
	if err == nil || !strings.Contains(err.Error(), "invalid --pos format") {
		t.Fatalf("lookup-column --pos 1,2 error = %v, want invalid --pos format", err)
	}
	if *calls != 0 {
		t.Fatalf("invalid --pos created %d data sources, want none", *calls)
	}
}

func TestLookupColumnRootIDOutput(t *testing.T) {
	data := &fakeRootInfoDataSource{
		row: &seatable.NeuronInfoRow{RootID: "111", X: 1, Y: 2, Z: 3, PositionSet: true},
		positions: []seatable.NeuronPositionRow{
			{RootID: "far", Region: "LX", X: 100, Y: 2, Z: 3, PositionSet: true},
			{RootID: "222", Region: "CX", X: 1, Y: 2, Z: 3, PositionSet: true},
		},
	}
	stubLookupColumnDataSource(t, data)

	out, _, err := runLookupColumnForTest(t, "111")
	if err != nil {
		t.Fatalf("lookup-column 111 error = %v", err)
	}
	if !strings.HasSuffix(out, "CX\t222\n") {
		t.Fatalf("lookup-column 111 output = %q, want the nearest region and root ID", out)
	}
}

func TestLookupColumnRootIDErrors(t *testing.T) {
	tests := []struct {
		name    string
		row     *seatable.NeuronInfoRow
		wantErr string
	}{
		{name: "unknown root ID", wantErr: `no neuron found with root_id "111"`},
		{
			name:    "neuron without position",
			row:     &seatable.NeuronInfoRow{RootID: "111"},
			wantErr: "neuron 111 has no position coordinates",
		},
		{
			name:    "no EPG/PEG candidates",
			row:     &seatable.NeuronInfoRow{RootID: "111", X: 1, Y: 2, Z: 3, PositionSet: true},
			wantErr: "no EPG/PEG neurons found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubLookupColumnDataSource(t, &fakeRootInfoDataSource{row: tt.row})
			_, _, err := runLookupColumnForTest(t, "111")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("lookup-column 111 error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
