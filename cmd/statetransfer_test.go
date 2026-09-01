package cmd

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"
)

func TestParseIDs(t *testing.T) {
	got := parseIDs("111, 222\n111\t333")
	want := []string{"111", "222", "333"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIDs() = %#v, want %#v", got, want)
	}
}

// stateTransferRun records what an isolated state-transfer run did with the
// outside world it can no longer reach.
type stateTransferRun struct {
	clipboard    string
	clipboardErr error
	layer        map[string]interface{}
	result       *nglstate.LoadResult

	wrote      bool
	wroteTo    string
	queriedIDs []string
}

// isolateStateTransferRun swaps every seam the command reaches the clipboard,
// SeaTable, and the filesystem through, so RunE can run without a desktop
// session or a token. The SeaTable stubs fail the test when they are called,
// so a run without --labels is checked never to query rather than assumed not
// to; withLabels replaces them for the runs that should.
func isolateStateTransferRun(t *testing.T) *stateTransferRun {
	t.Helper()

	previousRead := stClipboardRead
	previousNewClient := stNewClient
	previousQuery := stQueryNeuronsByRootIDs
	previousLoad := stLoadState
	previousFind := stFindSegmentationLayer
	previousWrite := stWriteState
	t.Cleanup(func() {
		stClipboardRead = previousRead
		stNewClient = previousNewClient
		stQueryNeuronsByRootIDs = previousQuery
		stLoadState = previousLoad
		stFindSegmentationLayer = previousFind
		stWriteState = previousWrite
	})

	run := &stateTransferRun{
		clipboard: "100 200",
		layer:     map[string]interface{}{"type": "segmentation", "source": "graphene://x"},
	}
	// Deliberately not the template, so forcing the source to template before
	// writing is observable.
	run.result = &nglstate.LoadResult{
		State:  map[string]interface{}{"layers": []interface{}{run.layer}},
		Source: nglstate.SourceFile,
	}

	stClipboardRead = func() (string, error) { return run.clipboard, run.clipboardErr }
	stLoadState = func(string, bool) (*nglstate.LoadResult, error) { return run.result, nil }
	stFindSegmentationLayer = func(map[string]interface{}, string) (map[string]interface{}, int, error) {
		return run.layer, 0, nil
	}
	stWriteState = func(got *nglstate.LoadResult, output string) error {
		if got != run.result {
			t.Fatalf("wrote a different result than the one loaded: %#v", got)
		}
		run.wrote, run.wroteTo = true, output
		return nil
	}
	stNewClient = func() (*seatable.Client, error) {
		t.Error("state-transfer built a SeaTable client without --labels")
		return nil, errors.New("unexpected SeaTable client")
	}
	stQueryNeuronsByRootIDs = func(*seatable.Client, []string) ([]seatable.NeuronRow, error) {
		t.Error("state-transfer queried SeaTable without --labels")
		return nil, errors.New("unexpected SeaTable query")
	}

	return run
}

// withLabels stubs the SeaTable seams for a run that passes --labels.
func (r *stateTransferRun) withLabels(rows []seatable.NeuronRow, queryErr error) {
	stNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
	stQueryNeuronsByRootIDs = func(_ *seatable.Client, ids []string) ([]seatable.NeuronRow, error) {
		r.queriedIDs = ids
		return rows, queryErr
	}
}

// TestStateTransferInjectsClipboardIDs covers the path from clipboard to
// written state: the IDs are parsed and deduped into the segmentation layer,
// and the source is forced to template so the result leaves as a URL rather
// than overwriting whatever the base state was read from.
func TestStateTransferInjectsClipboardIDs(t *testing.T) {
	run := isolateStateTransferRun(t)
	run.clipboard = "100, 200\n100\t300"

	if err := stateTransferCmd.RunE(stateTransferCmd, nil); err != nil {
		t.Fatal(err)
	}

	want := []interface{}{"100", "200", "300"}
	if got := run.layer["segments"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
	if !run.wrote {
		t.Fatal("state was never written")
	}
	if run.wroteTo != "" {
		t.Fatalf("wrote to %q, want the clipboard when --output is unset", run.wroteTo)
	}
	if run.result.Source != nglstate.SourceTemplate {
		t.Fatalf("source = %q, want %q so the URL goes to the clipboard", run.result.Source, nglstate.SourceTemplate)
	}
}

// TestStateTransferOutputFileKeepsStateSource pins the other half of that
// rewrite: an explicit --output is honored and the loaded source is left alone.
func TestStateTransferOutputFileKeepsStateSource(t *testing.T) {
	run := isolateStateTransferRun(t)
	out := filepath.Join(t.TempDir(), "out.json")
	setFlagForTest(t, stateTransferCmd, "output", out)

	if err := stateTransferCmd.RunE(stateTransferCmd, nil); err != nil {
		t.Fatal(err)
	}
	if run.wroteTo != out {
		t.Fatalf("wrote to %q, want the --output path %q", run.wroteTo, out)
	}
	if run.result.Source != nglstate.SourceFile {
		t.Fatalf("source = %q, want it untouched when --output is set", run.result.Source)
	}
}

func TestStateTransferClipboardFailures(t *testing.T) {
	tests := []struct {
		name      string
		clip      string
		clipErr   error
		wantError string
	}{
		{
			name:      "read fails",
			clipErr:   errors.New("no selection owner"),
			wantError: "reading clipboard",
		},
		{
			name:      "clipboard holds only whitespace",
			clip:      "  \n\t ",
			wantError: "clipboard is empty",
		},
		{
			name:      "clipboard holds only separators",
			clip:      ", ,\t,",
			wantError: "no valid IDs found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := isolateStateTransferRun(t)
			run.clipboard, run.clipboardErr = tt.clip, tt.clipErr

			err := stateTransferCmd.RunE(stateTransferCmd, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantError)
			}
			if run.wrote {
				t.Fatal("no state should be written when the clipboard yields no IDs")
			}
		})
	}
}

// TestStateTransferPublishesChosenLabelFields is the state-transfer half of the
// wiring add already covers: the command's own --label-by and --label-tags
// reach what gets published, and the clipboard IDs are what it looks up.
func TestStateTransferPublishesChosenLabelFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	run := isolateStateTransferRun(t)
	run.withLabels([]seatable.NeuronRow{
		{RootID: "100", CellType: "PFN", CellSubtype: "PFNc", Side: "left"},
		{RootID: "200", CellType: "PEN", Side: "right"},
	}, nil)

	published := filepath.Join(t.TempDir(), "info.json")
	setFlagForTest(t, stateTransferCmd, "labels", "true")
	setFlagForTest(t, stateTransferCmd, "labels-hook",
		labelsHookScript(t, published, "https://hook.example/new/|neuroglancer-precomputed:"))
	setFlagForTest(t, stateTransferCmd, "label-by", "cell_subtype,cell_type")
	setFlagForTest(t, stateTransferCmd, "label-tags", "side")

	if err := stateTransferCmd.RunE(stateTransferCmd, nil); err != nil {
		t.Fatal(err)
	}

	if want := []string{"100", "200"}; !reflect.DeepEqual(run.queriedIDs, want) {
		t.Fatalf("queried %v, want the clipboard root IDs %v", run.queriedIDs, want)
	}

	labels, tags := publishedLabelsAndTags(t, published)
	// cell_subtype labels the first row and falls back to cell_type for the
	// second, and only the requested side field becomes a filterable chip.
	if want := []string{"PFNc", "PEN"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
	if want := []string{"side_left", "side_right"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
}

func TestStateTransferLabelsNeedMetadata(t *testing.T) {
	tests := []struct {
		name      string
		rows      []seatable.NeuronRow
		queryErr  error
		wantError string
	}{
		{
			name:      "query fails",
			queryErr:  errors.New("401 unauthorized"),
			wantError: "querying labels for clipboard root IDs",
		},
		{
			name:      "no rows match",
			wantError: "no CRANT metadata found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := isolateStateTransferRun(t)
			run.withLabels(tt.rows, tt.queryErr)
			setFlagForTest(t, stateTransferCmd, "labels", "true")

			err := stateTransferCmd.RunE(stateTransferCmd, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantError)
			}
			if run.wrote {
				t.Fatal("a failed labels lookup should not write a state")
			}
		})
	}
}

// TestStateTransferWarnsOnPartialMetadata: a clipboard ID with no CRANT row is
// not fatal. The run warns and publishes the rows it did find, since a scene
// labeled for most of its segments beats no scene at all.
func TestStateTransferWarnsOnPartialMetadata(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	run := isolateStateTransferRun(t)
	run.clipboard = "100 200"
	run.withLabels([]seatable.NeuronRow{{RootID: "100", CellType: "PFN"}}, nil)

	published := filepath.Join(t.TempDir(), "info.json")
	setFlagForTest(t, stateTransferCmd, "labels", "true")
	setFlagForTest(t, stateTransferCmd, "labels-hook",
		labelsHookScript(t, published, "https://hook.example/new/|neuroglancer-precomputed:"))

	if err := stateTransferCmd.RunE(stateTransferCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !run.wrote {
		t.Fatal("a partial metadata match should still write the state")
	}
	if labels, _ := publishedLabelsAndTags(t, published); !reflect.DeepEqual(labels, []string{"PFN"}) {
		t.Fatalf("labels = %v, want just the row that matched", labels)
	}
}

func TestStateTransferRejectsBadColor(t *testing.T) {
	run := isolateStateTransferRun(t)
	setFlagForTest(t, stateTransferCmd, "color", "#bad")

	err := stateTransferCmd.RunE(stateTransferCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid color") {
		t.Fatalf("error = %v, want an invalid color error", err)
	}
	if run.wrote {
		t.Fatal("an invalid --color should not write a state")
	}
}

// TestStateTransferSeamFailuresSurface checks that each thing the command
// cannot do itself -- load a state, find a layer, reach SeaTable, write the
// result -- is reported rather than swallowed.
func TestStateTransferSeamFailuresSurface(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		labels bool
		fail   func(*stateTransferRun)
	}{
		{
			name: "state cannot be loaded",
			fail: func(*stateTransferRun) {
				stLoadState = func(string, bool) (*nglstate.LoadResult, error) { return nil, boom }
			},
		},
		{
			name: "no segmentation layer",
			fail: func(*stateTransferRun) {
				stFindSegmentationLayer = func(map[string]interface{}, string) (map[string]interface{}, int, error) {
					return nil, 0, boom
				}
			},
		},
		{
			name:   "SeaTable client cannot be built",
			labels: true,
			fail: func(r *stateTransferRun) {
				r.withLabels(nil, nil)
				stNewClient = func() (*seatable.Client, error) { return nil, boom }
			},
		},
		{
			name: "state cannot be written",
			fail: func(*stateTransferRun) {
				stWriteState = func(*nglstate.LoadResult, string) error { return boom }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := isolateStateTransferRun(t)
			if tt.labels {
				setFlagForTest(t, stateTransferCmd, "labels", "true")
			}
			tt.fail(run)

			if err := stateTransferCmd.RunE(stateTransferCmd, nil); err == nil || !strings.Contains(err.Error(), "boom") {
				t.Fatalf("error = %v, want it to carry the underlying failure", err)
			}
		})
	}
}

// TestStateTransferReportsLabelPublishFailure: labels that cannot be published
// fail the run rather than quietly handing back a state whose Seg. panel would
// show bare root IDs.
func TestStateTransferReportsLabelPublishFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	run := isolateStateTransferRun(t)
	run.withLabels([]seatable.NeuronRow{
		{RootID: "100", CellType: "PFN"},
		{RootID: "200", CellType: "PEN"},
	}, nil)

	setFlagForTest(t, stateTransferCmd, "labels", "true")
	setFlagForTest(t, stateTransferCmd, "labels-hook", "sh -c 'exit 3'")

	if err := stateTransferCmd.RunE(stateTransferCmd, nil); err == nil {
		t.Fatal("a labels hook that exits non-zero should fail the run")
	}
	if run.wrote {
		t.Fatal("no state should be written when its labels could not be published")
	}
}

// TestStateTransferPreRunNeedsTokenForLabels covers the precondition that keeps
// a --labels run from reaching the clipboard only to fail on an absent token.
func TestStateTransferPreRunNeedsTokenForLabels(t *testing.T) {
	tests := []struct {
		name      string
		labels    bool
		token     string
		wantError string
	}{
		{name: "without --labels no token is needed"},
		{name: "--labels with a stored token", labels: true, token: "secret"},
		{name: "--labels without a token", labels: true, wantError: "crantcli setup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := getAPIToken
			getAPIToken = func() string { return tt.token }
			t.Cleanup(func() { getAPIToken = previous })
			if tt.labels {
				setFlagForTest(t, stateTransferCmd, "labels", "true")
			}

			err := stateTransferCmd.PreRunE(stateTransferCmd, nil)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("PreRunE returned error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantError)
			}
		})
	}
}

// TestStateTransferRejectsBadLabelFields pins the same promise add makes: a
// mistyped field is caught before the command reads the clipboard or queries
// anything.
func TestStateTransferRejectsBadLabelFields(t *testing.T) {
	isolateStateTransferRun(t)
	stClipboardRead = func() (string, error) {
		t.Error("a mistyped --label-by should fail before the clipboard is read")
		return "", errors.New("unexpected clipboard read")
	}
	setFlagForTest(t, stateTransferCmd, "label-by", "cell_typo")

	want := `invalid --label-by "cell_typo"`
	if err := stateTransferCmd.PreRunE(stateTransferCmd, nil); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("PreRunE error = %v, want it to contain %q", err, want)
	}
	if err := stateTransferCmd.RunE(stateTransferCmd, nil); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("RunE error = %v, want it to contain %q", err, want)
	}
}

func TestStateTransferLabelsFlagsMatchAdd(t *testing.T) {
	for _, name := range []string{"labels", "labels-ttl", "labels-hook", "label-by", "label-tags"} {
		stateFlag := stateTransferCmd.Flags().Lookup(name)
		if stateFlag == nil {
			t.Fatalf("state-transfer is missing --%s", name)
		}
		addFlag := addCmd.Flags().Lookup(name)
		if addFlag == nil {
			t.Fatalf("add is missing --%s", name)
		}
		if stateFlag.DefValue != addFlag.DefValue {
			t.Fatalf("state-transfer --%s default = %q, want add default %q", name, stateFlag.DefValue, addFlag.DefValue)
		}
	}
}
