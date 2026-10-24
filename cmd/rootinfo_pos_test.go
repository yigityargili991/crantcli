package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

// newRootInfoArgsCmd builds a minimal command with the --pos flag so
// rootInfoArgs can be exercised without touching the registered command.
func newRootInfoArgsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "root-info [root_id]"}
	cmd.Flags().String("pos", "", "")
	return cmd
}

func TestRootInfoArgsRequiresExactlyOne(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		pos     string
		setPos  bool
		wantErr string
	}{
		{name: "root id only", args: []string{"576460752669867599"}},
		{name: "pos only", pos: "100.5,200.3,300.1", setPos: true},
		{name: "neither", wantErr: "provide a root_id argument or use --pos x,y,z"},
		{name: "both", args: []string{"576460752669867599"}, pos: "100.5,200.3,300.1", setPos: true,
			wantErr: "cannot combine root_id argument with --pos flag"},
		{name: "two args", args: []string{"111", "222"}, wantErr: "accepts at most 1 arg(s), received 2"},
		{name: "two args and pos", args: []string{"111", "222"}, pos: "1,2,3", setPos: true,
			wantErr: "accepts at most 1 arg(s), received 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootInfoArgsCmd()
			if tt.setPos {
				if err := cmd.Flags().Set("pos", tt.pos); err != nil {
					t.Fatalf("setting --pos: %v", err)
				}
			}
			err := rootInfoArgs(cmd, tt.args)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("rootInfoArgs(%v) error = %v, want nil", tt.args, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("rootInfoArgs(%v) error = %v, want %q", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestRunRootInfoPositionRejectsInvalidCoordinates(t *testing.T) {
	for _, pos := range []string{
		"1,2,nan",
		"1,NaN,3",
		"1,Inf,3",
		"-Inf,2,3",
		"1,2",
		"1,2,3,4",
		"a,b,c",
	} {
		t.Run(pos, func(t *testing.T) {
			var out bytes.Buffer
			err := runRootInfo(&out, &fakeRootInfoDataSource{}, nil, "", rootInfoOptions{Pos: pos})
			if err == nil || !strings.Contains(err.Error(), "invalid --pos format") {
				t.Fatalf("runRootInfo(--pos %q) error = %v, want invalid --pos format", pos, err)
			}
			if out.Len() != 0 {
				t.Fatalf("runRootInfo(--pos %q) wrote output: %q", pos, out.String())
			}
		})
	}
}

func positionTestDataSource() *fakeRootInfoDataSource {
	return &fakeRootInfoDataSource{
		regionOpts: map[string]string{"CX": "1", "LX": "2"},
		positions: []seatable.NeuronPositionRow{
			{RootID: "far", Region: "LX", Side: "right", X: 0, Y: 0, Z: 0, PositionSet: true},
			{RootID: "222", Region: "CX", Side: "left", X: 100.5, Y: 200.3, Z: 300.1, PositionSet: true},
		},
	}
}

func TestRunRootInfoPositionText(t *testing.T) {
	var out bytes.Buffer
	if err := runRootInfo(&out, positionTestDataSource(), nil, "", rootInfoOptions{Pos: "100.5,200.3,300.1"}); err != nil {
		t.Fatalf("runRootInfo(--pos) error = %v", err)
	}

	want := "nearest_column\n" +
		"  region: CX\n" +
		"  root_id: 222\n" +
		"  side: left\n" +
		"  distance: 0.000\n"
	if got := out.String(); got != want {
		t.Fatalf("--pos text output = %q, want %q", got, want)
	}
	if strings.Contains(out.String(), "side_relation") {
		t.Fatalf("--pos text output has a side relation: %q", out.String())
	}
}

func TestRunRootInfoPositionJSON(t *testing.T) {
	var out bytes.Buffer
	if err := runRootInfo(&out, positionTestDataSource(), nil, "", rootInfoOptions{Pos: "100.5,200.3,300.1", JSON: true}); err != nil {
		t.Fatalf("runRootInfo(--pos --json) error = %v", err)
	}

	want := `{
  "nearest_column": {
    "region": "CX",
    "root_id": "222",
    "side": "left",
    "distance": 0
  }
}
`
	if got := out.String(); got != want {
		t.Fatalf("--pos JSON output = %q, want %q", got, want)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
		t.Fatalf("decoding --pos JSON: %v", err)
	}
	if len(fields) != 1 || fields["nearest_column"] == nil {
		t.Fatalf("--pos JSON top-level fields = %v, want only nearest_column", fields)
	}
	var nearest map[string]json.RawMessage
	if err := json.Unmarshal(fields["nearest_column"], &nearest); err != nil {
		t.Fatalf("decoding nearest_column: %v", err)
	}
	if _, ok := nearest["side_relation"]; ok {
		t.Fatalf("--pos JSON has a side relation: %s", out.String())
	}
}

func TestRunRootInfoPositionReportsMissingCandidates(t *testing.T) {
	data := &fakeRootInfoDataSource{regionOpts: map[string]string{}}
	var out bytes.Buffer
	err := runRootInfo(&out, data, nil, "", rootInfoOptions{Pos: "1,2,3"})
	if err == nil || !strings.Contains(err.Error(), "no EPG/PEG neurons found") {
		t.Fatalf("runRootInfo(--pos) error = %v, want missing candidates", err)
	}
}
