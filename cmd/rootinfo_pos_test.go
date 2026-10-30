// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

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
	if err := runRootInfoPosition(&out, positionTestDataSource(), 100.5, 200.3, 300.1, rootInfoOptions{}); err != nil {
		t.Fatalf("runRootInfoPosition error = %v", err)
	}

	want := "nearest_column\n" +
		"  region: CX\n" +
		"  root_id: 222\n" +
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
	if err := runRootInfoPosition(&out, positionTestDataSource(), 100.5, 200.3, 300.1, rootInfoOptions{JSON: true}); err != nil {
		t.Fatalf("runRootInfoPosition error = %v", err)
	}

	want := `{
  "nearest_column": {
    "region": "CX",
    "root_id": "222",
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
	for _, field := range []string{"side", "side_relation"} {
		if _, ok := nearest[field]; ok {
			t.Fatalf("--pos JSON has %q: %s", field, out.String())
		}
	}
}

// The full root-ID report annotates its nearest column with side and
// side_relation; a --pos lookup has no side relation, so the standalone output
// keeps printing only region, root_id, and distance.
func TestRunRootInfoPositionOmitsSide(t *testing.T) {
	data := &fakeRootInfoDataSource{
		regionOpts: map[string]string{"CX": "1"},
		positions: []seatable.NeuronPositionRow{
			{RootID: "222", Region: "CX", Side: "left", X: 1, Y: 2, Z: 3, PositionSet: true},
		},
	}

	wantText := "nearest_column\n" +
		"  region: CX\n" +
		"  root_id: 222\n" +
		"  distance: 0.000\n"
	var out bytes.Buffer
	if err := runRootInfoPosition(&out, data, 1, 2, 3, rootInfoOptions{}); err != nil {
		t.Fatalf("runRootInfoPosition error = %v", err)
	}
	if got := out.String(); got != wantText {
		t.Fatalf("--pos text output = %q, want %q", got, wantText)
	}

	var jsonOut bytes.Buffer
	if err := runRootInfoPosition(&jsonOut, data, 1, 2, 3, rootInfoOptions{JSON: true}); err != nil {
		t.Fatalf("runRootInfoPosition --json error = %v", err)
	}
	if strings.Contains(jsonOut.String(), `"side"`) || strings.Contains(jsonOut.String(), "side_relation") {
		t.Fatalf("--pos JSON has a side field: %s", jsonOut.String())
	}
	if strings.Contains(jsonOut.String(), `"left"`) {
		t.Fatalf("--pos JSON prints the nearest column's side: %s", jsonOut.String())
	}
}

func TestRunRootInfoPositionReportsMissingCandidates(t *testing.T) {
	data := &fakeRootInfoDataSource{regionOpts: map[string]string{}}
	var out bytes.Buffer
	err := runRootInfoPosition(&out, data, 1, 2, 3, rootInfoOptions{})
	if err == nil || !strings.Contains(err.Error(), "no EPG/PEG neurons found") {
		t.Fatalf("runRootInfoPosition error = %v, want missing candidates", err)
	}
}

// rootInfoCLIStubs counts how often the command factories run, so tests can
// assert which clients a mode creates.
type rootInfoCLIStubs struct {
	dataCalls int
	caveCalls int
}

// stubRootInfoCommandClients replaces the root-info client factories and token
// lookup for one CLI-level test, and restores them afterwards.
func stubRootInfoCommandClients(t *testing.T, data rootInfoDataSource) *rootInfoCLIStubs {
	t.Helper()

	stubs := &rootInfoCLIStubs{}
	previousDataSource := rootInfoNewDataSource
	previousCAVE := rootInfoNewCAVEClient
	previousGetAPIToken := getAPIToken
	t.Cleanup(func() {
		rootInfoNewDataSource = previousDataSource
		rootInfoNewCAVEClient = previousCAVE
		getAPIToken = previousGetAPIToken
	})

	getAPIToken = func() string { return "test-token" }
	rootInfoNewDataSource = func() (rootInfoDataSource, error) {
		stubs.dataCalls++
		return data, nil
	}
	rootInfoNewCAVEClient = func() (rootInfoCaveClient, error) {
		stubs.caveCalls++
		return &fakeRootInfoCaveClient{}, nil
	}
	return stubs
}

// runRootInfoForTest runs the registered root-info command with clean flags.
func runRootInfoForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	resetCommandFlags(rootInfoCmd)
	t.Cleanup(func() { resetCommandFlags(rootInfoCmd) })
	return executeRootForTest(t, append([]string{"root-info"}, args...)...)
}

func TestRootInfoCommandPositionText(t *testing.T) {
	stubs := stubRootInfoCommandClients(t, positionTestDataSource())
	out, _, err := runRootInfoForTest(t, "--pos", "100.5,200.3,300.1")
	if err != nil {
		t.Fatalf("root-info --pos error = %v", err)
	}

	want := "nearest_column\n" +
		"  region: CX\n" +
		"  root_id: 222\n" +
		"  distance: 0.000\n"
	if out != want {
		t.Fatalf("root-info --pos output = %q, want %q", out, want)
	}
	if stubs.dataCalls != 1 {
		t.Fatalf("data source factory calls = %d, want 1", stubs.dataCalls)
	}
	if stubs.caveCalls != 0 {
		t.Fatalf("CAVE client factory calls = %d, want 0 for --pos", stubs.caveCalls)
	}
}

func TestRootInfoCommandPositionJSON(t *testing.T) {
	stubs := stubRootInfoCommandClients(t, positionTestDataSource())
	out, _, err := runRootInfoForTest(t, "--pos", "100.5,200.3,300.1", "--json")
	if err != nil {
		t.Fatalf("root-info --pos --json error = %v", err)
	}

	want := `{
  "nearest_column": {
    "region": "CX",
    "root_id": "222",
    "distance": 0
  }
}
`
	if out != want {
		t.Fatalf("root-info --pos --json output = %q, want %q", out, want)
	}
	if stubs.caveCalls != 0 {
		t.Fatalf("CAVE client factory calls = %d, want 0 for --pos --json", stubs.caveCalls)
	}
}

func TestRootInfoCommandPositionInvalidFailsBeforeClients(t *testing.T) {
	stubs := stubRootInfoCommandClients(t, positionTestDataSource())
	for _, pos := range []string{"1,2,nan", "1,2", "a,b,c"} {
		t.Run(pos, func(t *testing.T) {
			out, _, err := runRootInfoForTest(t, "--pos", pos)
			if err == nil || !strings.Contains(err.Error(), "invalid --pos format") {
				t.Fatalf("root-info --pos %q error = %v, want invalid --pos format", pos, err)
			}
			if out != "" {
				t.Fatalf("root-info --pos %q wrote output: %q", pos, out)
			}
		})
	}
	if stubs.dataCalls != 0 || stubs.caveCalls != 0 {
		t.Fatalf("invalid --pos created clients: data=%d cave=%d", stubs.dataCalls, stubs.caveCalls)
	}
}

func TestRootInfoCommandPositionRejectsEmpty(t *testing.T) {
	stubs := stubRootInfoCommandClients(t, positionTestDataSource())
	for _, args := range [][]string{{"--pos", ""}, {"--pos="}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, _, err := runRootInfoForTest(t, args...)
			if err == nil || !strings.Contains(err.Error(), "--pos requires a position as x,y,z") {
				t.Fatalf("root-info %v error = %v, want clear --pos error", args, err)
			}
			if out != "" {
				t.Fatalf("root-info %v wrote output: %q", args, out)
			}
		})
	}
	if stubs.dataCalls != 0 || stubs.caveCalls != 0 {
		t.Fatalf("empty --pos created clients: data=%d cave=%d", stubs.dataCalls, stubs.caveCalls)
	}
}

func TestRootInfoCommandRejectsArgumentWithPosition(t *testing.T) {
	stubs := stubRootInfoCommandClients(t, positionTestDataSource())
	_, _, err := runRootInfoForTest(t, "111", "--pos", "1,2,3")
	if err == nil || !strings.Contains(err.Error(), "cannot combine root_id argument with --pos flag") {
		t.Fatalf("root-info 111 --pos error = %v, want combination error", err)
	}
	if stubs.dataCalls != 0 || stubs.caveCalls != 0 {
		t.Fatalf("combined root_id and --pos created clients: data=%d cave=%d", stubs.dataCalls, stubs.caveCalls)
	}
}

func TestRootInfoCommandPositionRejectsRootIDOnlyFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "history-limit",
			args:    []string{"--pos", "1,2,3", "--history-limit=-1"},
			wantErr: "cannot combine --pos with --history-limit",
		},
		{
			name:    "unfiltered",
			args:    []string{"--pos", "1,2,3", "--unfiltered"},
			wantErr: "cannot combine --pos with --unfiltered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubs := stubRootInfoCommandClients(t, positionTestDataSource())
			_, _, err := runRootInfoForTest(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("root-info %v error = %v, want %q", tt.args, err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "--history-limit must be >= 0") {
				t.Fatalf("root-info %v hit the root-ID-only history check: %v", tt.args, err)
			}
			if stubs.dataCalls != 0 || stubs.caveCalls != 0 {
				t.Fatalf("root-info %v created clients: data=%d cave=%d", tt.args, stubs.dataCalls, stubs.caveCalls)
			}
		})
	}
}

func TestRootInfoCommandRootIDCreatesCAVEClient(t *testing.T) {
	data := &fakeRootInfoDataSource{
		row: &seatable.NeuronInfoRow{RootID: "111", ExtraFields: map[string]string{}},
	}
	stubs := stubRootInfoCommandClients(t, data)
	out, _, err := runRootInfoForTest(t, "111")
	if err != nil {
		t.Fatalf("root-info 111 error = %v", err)
	}
	if !strings.Contains(out, "root_id: 111") {
		t.Fatalf("root-info 111 output = %q, want root_id", out)
	}
	if stubs.dataCalls != 1 || stubs.caveCalls != 1 {
		t.Fatalf("root-ID mode factory calls = data %d cave %d, want 1 and 1", stubs.dataCalls, stubs.caveCalls)
	}
}

func TestRootInfoCommandRejectsNegativeHistoryLimit(t *testing.T) {
	stubs := stubRootInfoCommandClients(t, positionTestDataSource())
	_, _, err := runRootInfoForTest(t, "111", "--history-limit=-1")
	if err == nil || !strings.Contains(err.Error(), "--history-limit must be >= 0") {
		t.Fatalf("root-info 111 --history-limit=-1 error = %v, want negative limit error", err)
	}
	if stubs.dataCalls != 0 || stubs.caveCalls != 0 {
		t.Fatalf("negative --history-limit created clients: data=%d cave=%d", stubs.dataCalls, stubs.caveCalls)
	}
}
