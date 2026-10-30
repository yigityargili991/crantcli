// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"crantcli/internal/cave"
	"crantcli/internal/seatable"

	"github.com/spf13/pflag"
)

// fakeCAVE serves CAVE's single and batch root lookups from resolve.
func fakeCAVE(t *testing.T, resolve func(sv uint64) uint64) *cave.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/roots_binary") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			out := make([]byte, len(body))
			for i := 0; i+8 <= len(body); i += 8 {
				binary.LittleEndian.PutUint64(out[i:], resolve(binary.LittleEndian.Uint64(body[i:])))
			}
			w.Write(out)
			return
		}
		// .../node/<supervoxel>/root
		parts := strings.Split(r.URL.Path, "/")
		sv, err := strconv.ParseUint(parts[len(parts)-2], 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"root_id": %d}`, resolve(sv))
	}))
	t.Cleanup(srv.Close)
	return cave.NewTestClient(srv.URL, srv.Client())
}

// isolateCheckCaveRun resets check-cave's flags and replaces its SeaTable
// query and CAVE client for a test that runs the command.
func isolateCheckCaveRun(t *testing.T, query func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error), caveClient *cave.Client) {
	t.Helper()
	resetFlags := func() {
		checkCaveCmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	previousSeaTable := checkCaveNewSeaTableClient
	previousCAVE := checkCaveNewCAVEClient
	previousQuery := checkCaveQueryNeurons
	previousToken := getAPIToken
	t.Cleanup(func() {
		resetFlags()
		checkCaveNewSeaTableClient = previousSeaTable
		checkCaveNewCAVEClient = previousCAVE
		checkCaveQueryNeurons = previousQuery
		getAPIToken = previousToken
	})

	resetFlags()
	getAPIToken = func() string { return "test-token" }
	checkCaveNewSeaTableClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
	checkCaveNewCAVEClient = func() (*cave.Client, error) { return caveClient, nil }
	checkCaveQueryNeurons = query
}

func writeCheckCaveState(t *testing.T, segments ...interface{}) string {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{
		"layers": []interface{}{
			map[string]interface{}{"name": "seg", "type": "segmentation", "segments": segments},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readCheckCaveSegments(t *testing.T, path string) []interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Layers []struct {
			Segments []interface{} `json:"segments"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state.Layers[0].Segments
}

func TestCheckNeurons_SingleOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"root_id": 999}`)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: "100"},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Status != statusOK {
		t.Errorf("status = %q, want %s", results[0].Status, statusOK)
	}
}

func TestStaleRootMappings(t *testing.T) {
	results := []checkResult{
		{RootID: "111", CaveRootID: "111", Status: statusOK},
		{RootID: "222", CaveRootID: "333", Status: statusStale},
		{RootID: "444", CaveRootID: "-", Status: statusError},
		{RootID: "555", CaveRootID: "666", Status: statusStale},
		// A duplicate row adds nothing; a split root maps to each current root.
		{RootID: "222", CaveRootID: "333", Status: statusStale},
		{RootID: "555", CaveRootID: "777", Status: statusStale},
	}

	got := staleRootMappings(results)
	want := []rootMapping{
		{OldRootID: "222", CurrentRootID: "333"},
		{OldRootID: "555", CurrentRootID: "666"},
		{OldRootID: "555", CurrentRootID: "777"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d mappings, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mapping[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestWriteMappings(t *testing.T) {
	var buf bytes.Buffer
	mappings := []rootMapping{
		{OldRootID: "222", CurrentRootID: "333"},
		{OldRootID: "555", CurrentRootID: "666"},
	}

	if err := writeMappings(&buf, mappings); err != nil {
		t.Fatalf("writeMappings: %v", err)
	}

	want := "222\t333\n555\t666\n"
	if got := buf.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestFailOnResultErrors(t *testing.T) {
	results := []checkResult{
		{RootID: "111", Status: statusOK},
		{RootID: "222", Status: statusError, Err: errors.New("transient failure")},
		{RootID: "333", Status: statusError, Err: errors.New("invalid root")},
	}

	var buf bytes.Buffer
	err := failOnResultErrors(&buf, results)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "2 of 3 checks failed; output incomplete"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}

	want := "  error for 222: transient failure\n  error for 333: invalid root\n"
	if got := buf.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestCheckNeurons_Stale(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"root_id": 888}`)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: "100"},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != statusStale {
		t.Errorf("status = %q, want %s", results[0].Status, statusStale)
	}
	if results[0].CaveRootID != "888" {
		t.Errorf("cave_root_id = %q, want 888", results[0].CaveRootID)
	}
}

func TestCheckNeurons_NoSupervoxel(t *testing.T) {
	c := cave.NewTestClient("http://unused", http.DefaultClient)

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: ""},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != statusNoSupervoxel {
		t.Errorf("status = %q, want %s", results[0].Status, statusNoSupervoxel)
	}
}

func TestCheckNeurons_InvalidSupervoxelID(t *testing.T) {
	c := cave.NewTestClient("http://unused", http.DefaultClient)

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: "not-a-number"},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != statusError {
		t.Errorf("status = %q, want %s", results[0].Status, statusError)
	}
	if results[0].Err == nil {
		t.Error("expected Err to be set for invalid supervoxel_id")
	}
}

func TestCheckNeurons_CaveAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: "100"},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != statusError {
		t.Errorf("status = %q, want %s", results[0].Status, statusError)
	}
}

func TestCheckNeurons_MixedStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"root_id": 1000}`)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "1000", SupervoxelID: "100"},
		{RootID: "2000", SupervoxelID: "200"},
		{RootID: "3000", SupervoxelID: ""},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != statusOK {
		t.Errorf("results[0].Status = %q, want %s", results[0].Status, statusOK)
	}
	if results[1].Status != statusStale {
		t.Errorf("results[1].Status = %q, want %s", results[1].Status, statusStale)
	}
	if results[2].Status != statusNoSupervoxel {
		t.Errorf("results[2].Status = %q, want %s", results[2].Status, statusNoSupervoxel)
	}
}

func TestCheckNeurons_BatchMode(t *testing.T) {
	neurons := make([]seatable.NeuronCaveCheckRow, 15)
	for i := range neurons {
		neurons[i] = seatable.NeuronCaveCheckRow{
			RootID:       fmt.Sprintf("%d", 1000+i),
			SupervoxelID: fmt.Sprintf("%d", 100+i),
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8*15)
		for i := range 15 {
			binary.LittleEndian.PutUint64(buf[i*8:], uint64(1000+i))
		}
		w.Write(buf)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	for i, r := range results {
		if r.Status != statusOK {
			t.Errorf("results[%d].Status = %q, want %s", i, r.Status, statusOK)
		}
	}
}

func TestCheckNeurons_UnresolvedSupervoxelIsAnError(t *testing.T) {
	// CAVE's batch endpoint answers 0 for a supervoxel it cannot resolve. A
	// root of 0, or one equal to the supervoxel, is a failed lookup and must
	// not read as a root that changed.
	resolve := func(sv uint64) uint64 {
		switch sv {
		case 100:
			return 0
		case 101:
			return 101
		default:
			return sv + 1000
		}
	}
	for _, n := range []int{2, batchThreshold + 1} {
		t.Run(fmt.Sprintf("%d neurons", n), func(t *testing.T) {
			neurons := make([]seatable.NeuronCaveCheckRow, n)
			for i := range neurons {
				neurons[i] = seatable.NeuronCaveCheckRow{RootID: fmt.Sprint(1100 + i), SupervoxelID: fmt.Sprint(100 + i)}
			}
			results, err := checkNeurons(fakeCAVE(t, resolve), neurons)
			if err != nil {
				t.Fatalf("checkNeurons: %v", err)
			}
			for _, r := range results[:2] {
				if r.Status != statusError || r.CaveRootID != "-" || r.Err == nil {
					t.Errorf("result %+v, want an error without a CAVE root", r)
				}
			}
			for _, r := range results[2:] {
				if r.Status != statusOK {
					t.Errorf("result %+v, want ok", r)
				}
			}
			if got := staleRootMappings(results); len(got) != 0 {
				t.Errorf("mappings = %v, want none", got)
			}
		})
	}
}

func TestCheckNeurons_NormalizesStoredIDs(t *testing.T) {
	results, err := checkNeurons(fakeCAVE(t, func(uint64) uint64 { return 888 }), []seatable.NeuronCaveCheckRow{
		{RootID: " 0999\n", SupervoxelID: " 100 "},
		{RootID: "777", SupervoxelID: " \t"},
	})
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if r := results[0]; r.Status != statusStale || r.RootID != "999" || r.SupervoxelID != "100" {
		t.Errorf("results[0] = %+v, want stale 999 with supervoxel 100", r)
	}
	if r := results[1]; r.Status != statusNoSupervoxel || r.Err != nil {
		t.Errorf("results[1] = %+v, want no_supervoxel", r)
	}
	// The mapping must name the ID the way a Neuroglancer state does.
	want := []rootMapping{{OldRootID: "999", CurrentRootID: "888"}}
	if got := staleRootMappings(results); !reflect.DeepEqual(got, want) {
		t.Errorf("mappings = %v, want %v", got, want)
	}
}

func TestRootMappingMapKeepsEverySplitRoot(t *testing.T) {
	got := rootMappingMap([]rootMapping{
		{OldRootID: "100", CurrentRootID: "201"},
		{OldRootID: "100", CurrentRootID: "202"},
		{OldRootID: "300", CurrentRootID: "301"},
	})
	want := map[string][]string{"100": {"201", "202"}, "300": {"301"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rootMappingMap = %v, want %v", got, want)
	}
}

func TestWriteResultErrorsNeutralizesControlCharacters(t *testing.T) {
	// root_id comes from SeaTable, which any collaborator can edit.
	results := []checkResult{{RootID: "\x1b]0;title\a\x1b[2J", Status: statusError, Err: errors.New("bad\x1b[31m")}}
	var buf bytes.Buffer
	if _, err := writeResultErrors(&buf, results); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(buf.String(), "\x1b\a") {
		t.Fatalf("error line kept control characters: %q", buf.String())
	}
}

func TestCanonicalRootIDs(t *testing.T) {
	got, err := canonicalRootIDs([]string{" 0123", "123", "456"})
	if err != nil || !reflect.DeepEqual(got, []string{"123", "456"}) {
		t.Fatalf("canonicalRootIDs = %q, %v; want [123 456]", got, err)
	}
	if _, err := canonicalRootIDs([]string{"123", "12a"}); err == nil || !strings.Contains(err.Error(), `invalid root_id "12a"`) {
		t.Fatalf("canonicalRootIDs error = %v, want invalid root_id", err)
	}
}

func TestQueryRootSupervoxels(t *testing.T) {
	previousQuery := checkCaveQueryNeurons
	t.Cleanup(func() { checkCaveQueryNeurons = previousQuery })
	var gotFilters *seatable.Filters
	checkCaveQueryNeurons = func(_ *seatable.Client, f *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error) {
		gotFilters = f
		// Table order; root 100 has two rows.
		return []seatable.NeuronCaveCheckRow{
			{RootID: "200", SupervoxelID: "3"},
			{RootID: "100", SupervoxelID: "1"},
			{RootID: "100", SupervoxelID: "2"},
		}, nil
	}

	got, err := queryRootSupervoxels(&seatable.Client{}, []string{"100", "200"})
	if err != nil {
		t.Fatalf("queryRootSupervoxels: %v", err)
	}
	want := []seatable.NeuronCaveCheckRow{
		{RootID: "100", SupervoxelID: "1"},
		{RootID: "100", SupervoxelID: "2"},
		{RootID: "200", SupervoxelID: "3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
	if gotFilters == nil || !reflect.DeepEqual(gotFilters.RootIDs, []string{"100", "200"}) {
		t.Errorf("query filters = %+v, want root IDs [100 200]", gotFilters)
	}

	if _, err := queryRootSupervoxels(&seatable.Client{}, []string{"100", "300"}); err == nil || err.Error() != `no neuron found with root_id "300"` {
		t.Errorf("missing root error = %v", err)
	}
}

func TestCheckCaveRefreshStateChecksLayerBeforeQuerying(t *testing.T) {
	queried := false
	isolateCheckCaveRun(t, func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error) {
		queried = true
		return nil, nil
	}, nil)
	statePath := writeCheckCaveState(t, "100")

	_, _, err := executeRootForTest(t, "check-cave", "--all", "--refresh-state", "-s", statePath, "--layer", "missing")
	if err == nil || !strings.Contains(err.Error(), `no segmentation layer named "missing"`) {
		t.Fatalf("error = %v, want missing layer", err)
	}
	if queried {
		t.Error("check-cave queried SeaTable before rejecting the layer")
	}
}

func TestCheckCaveRefreshStateWithoutMatchesWritesState(t *testing.T) {
	isolateCheckCaveRun(t, func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error) {
		return nil, nil
	}, nil)
	statePath := writeCheckCaveState(t, "100")
	outPath := filepath.Join(t.TempDir(), "out.json")

	if _, _, err := executeRootForTest(t, "check-cave", "--cell-type", "none", "--refresh-state", "-s", statePath, "-o", outPath); err != nil {
		t.Fatalf("check-cave: %v", err)
	}
	if got := readCheckCaveSegments(t, outPath); !reflect.DeepEqual(got, []interface{}{"100"}) {
		t.Errorf("segments = %v, want the state unchanged", got)
	}
}

func TestCheckCaveRefreshStateAddsEveryRootOfASplit(t *testing.T) {
	// Root 100 held two neurons; a split left each supervoxel in its own root.
	var gotFilters *seatable.Filters
	isolateCheckCaveRun(t, func(_ *seatable.Client, f *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error) {
		gotFilters = f
		return []seatable.NeuronCaveCheckRow{{RootID: "100", SupervoxelID: "1"}, {RootID: "100", SupervoxelID: "2"}}, nil
	}, fakeCAVE(t, func(sv uint64) uint64 { return 200 + sv }))
	statePath := writeCheckCaveState(t, "100", "300")
	outPath := filepath.Join(t.TempDir(), "out.json")

	if _, _, err := executeRootForTest(t, "check-cave", "100", "0100", "--refresh-state", "-s", statePath, "-o", outPath); err != nil {
		t.Fatalf("check-cave: %v", err)
	}
	if gotFilters == nil || !reflect.DeepEqual(gotFilters.RootIDs, []string{"100"}) {
		t.Errorf("query filters = %+v, want root IDs [100]", gotFilters)
	}
	if got, want := readCheckCaveSegments(t, outPath), []interface{}{"201", "202", "300"}; !reflect.DeepEqual(got, want) {
		t.Errorf("segments = %v, want %v", got, want)
	}
}

func TestCheckCaveFailedLookupsExitWithStatus2(t *testing.T) {
	// Root 1 is stale; supervoxel 20 cannot be resolved.
	rows := []seatable.NeuronCaveCheckRow{{RootID: "1", SupervoxelID: "10"}, {RootID: "2", SupervoxelID: "20"}}
	resolve := func(sv uint64) uint64 {
		if sv == 20 {
			return 0
		}
		return 5
	}
	for _, args := range [][]string{{"--all"}, {"--all", "--quiet"}, {"--all", "--mapping"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			isolateCheckCaveRun(t, func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error) {
				return rows, nil
			}, fakeCAVE(t, resolve))

			_, _, err := executeRootForTest(t, append([]string{"check-cave"}, args...)...)
			if err == nil || !strings.Contains(err.Error(), "1 of 2 checks failed") {
				t.Fatalf("error = %v, want a failed check", err)
			}
			if code := exitCode(checkCaveCmd, err); code != 2 {
				t.Errorf("exit status = %d, want 2", code)
			}
		})
	}
}

func TestCheckCaveQuietStaleExitsWithStatus1(t *testing.T) {
	isolateCheckCaveRun(t, func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error) {
		return []seatable.NeuronCaveCheckRow{{RootID: "1", SupervoxelID: "10"}}, nil
	}, fakeCAVE(t, func(uint64) uint64 { return 5 }))

	_, _, err := executeRootForTest(t, "check-cave", "--all", "--quiet")
	if code := exitCode(checkCaveCmd, err); code != 1 {
		t.Fatalf("exit status = %d (%v), want 1", code, err)
	}
}

func TestCheckCaveRejectsEmptyFilter(t *testing.T) {
	isolateCheckCaveRun(t, func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronCaveCheckRow, error) {
		t.Error("check-cave queried SeaTable with an empty filter")
		return nil, nil
	}, nil)

	_, _, err := executeRootForTest(t, "check-cave", "--all", "--region", " ")
	if err == nil || err.Error() != "--region cannot be empty" {
		t.Fatalf("error = %v, want --region cannot be empty", err)
	}
}

func TestCheckCaveFlagErrorExitsWithStatus2(t *testing.T) {
	isolateCheckCaveRun(t, nil, nil)
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"check-cave", "--no-such-flag"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})

	cmd, err := rootCmd.ExecuteC()
	if code := exitCode(cmd, err); err == nil || code != 2 {
		t.Fatalf("exit status = %d (%v), want 2", code, err)
	}
}
