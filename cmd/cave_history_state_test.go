package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"crantcli/internal/cave"
	"crantcli/internal/nglstate"
)

func historyStateFixture() []caveHistoryResult {
	return []caveHistoryResult{{RootID: "576460752688642351", Entries: caveRowsToHistoryEntries([]cave.ChangeLogRow{
		{OperationID: 1, Timestamp: 1700000000000, BeforeRootIDs: []uint64{10, 11}, AfterRootIDs: []uint64{12}, IsMerge: true, UserName: "Ada"},
		{OperationID: 2, Timestamp: 1700000001000, BeforeRootIDs: []uint64{12}, AfterRootIDs: []uint64{13, 14}, UserID: 42},
	})}}
}

func TestCaveHistoryStateStagesAndLabels(t *testing.T) {
	results := historyStateFixture()
	state, layers, info, err := buildCaveHistoryState(results, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 5 || len(state["layers"].([]interface{})) != 6 {
		t.Fatalf("unexpected layer counts: %d", len(layers))
	}
	wantSegments := [][]interface{}{{"576460752688642351"}, {"12"}, {"13", "14"}, {"10", "11"}, {"12"}}
	for i, layer := range layers {
		if !reflect.DeepEqual(layer["segments"], wantSegments[i]) {
			t.Errorf("layer %d segments = %v", i, layer["segments"])
		}
		if layer["visible"] != (i == 1) {
			t.Errorf("layer %d visibility = %v", i, layer["visible"])
		}
	}
	if !strings.Contains(layers[1]["name"].(string), "split #2 | user 42 | 2023-11-14T22:13:21Z | before") {
		t.Fatalf("layer name = %v", layers[1]["name"])
	}
	firstColors := layers[1]["segmentColors"].(map[string]interface{})
	lastColors := layers[4]["segmentColors"].(map[string]interface{})
	if firstColors["12"] != lastColors["12"] {
		t.Fatal("same historical root changes color across layers")
	}
	mergeColors := layers[3]["segmentColors"].(map[string]interface{})
	if mergeColors["10"] == mergeColors["11"] {
		t.Fatal("merge inputs have the same color")
	}
	var props struct {
		Type   string `json:"@type"`
		Inline struct {
			IDs        []string `json:"ids"`
			Properties []struct {
				Values []string `json:"values"`
			} `json:"properties"`
		} `json:"inline"`
	}
	if err := json.Unmarshal(info, &props); err != nil {
		t.Fatal(err)
	}
	if props.Type != "neuroglancer_segment_properties" {
		t.Fatal(props.Type)
	}
	found := false
	for i, id := range props.Inline.IDs {
		if id != "12" {
			continue
		}
		found = true
		label := props.Inline.Properties[0].Values[i]
		for _, want := range []string{"before split #2", "user 42", "after (reported) merge #1", "Ada", "2023-11-14T22:13:20Z"} {
			if !strings.Contains(label, want) {
				t.Errorf("label %q missing %q", label, want)
			}
		}
	}
	if !found {
		t.Fatal("shared historical root missing from labels")
	}
	if results[0].Entries[0].OperationID != 1 {
		t.Fatal("builder mutated input ordering")
	}
	url, err := nglstate.EncodeURL(state, "")
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := nglstate.DecodeURL(url)
	if err != nil {
		t.Fatal(err)
	}
	originalJSON, _ := json.Marshal(state)
	decodedJSON, _ := json.Marshal(roundTrip)
	if !bytes.Equal(originalJSON, decodedJSON) {
		t.Fatal("state did not round-trip through viewer URL")
	}
}

func TestCaveHistoryStateLimitAndEmptyHistory(t *testing.T) {
	results := historyStateFixture()
	results = append(results, caveHistoryResult{RootID: "99"}, results[0])
	_, layers, info, err := buildCaveHistoryState(results, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 4 {
		t.Fatalf("layers = %d, want latest edit pair and two queried roots", len(layers))
	}
	if layers[3]["visible"] != true {
		t.Fatal("root without history should be visible")
	}
	if strings.Contains(string(info), "Ada") {
		t.Fatal("omitted edit leaked into labels")
	}
}

func TestCompactCaveHistoryState(t *testing.T) {
	results := historyStateFixture()
	results = append(results, caveHistoryResult{RootID: "99"}, results[0])
	for _, tt := range []struct {
		name     string
		limit    int
		segments []interface{}
	}{
		{"all edits", 0, []interface{}{"!10", "!11", "12", "!13", "!14", "99", "!576460752688642351"}},
		{"latest edit", 1, []interface{}{"12", "!13", "!14", "99", "!576460752688642351"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state, layers, info, err := buildCaveHistoryState(results, tt.limit, true)
			if err != nil {
				t.Fatal(err)
			}
			if len(layers) != 1 || len(state["layers"].([]interface{})) != 2 {
				t.Fatalf("want image and one history layer, got %v", state["layers"])
			}
			layer := layers[0]
			if !reflect.DeepEqual(layer["segments"], tt.segments) {
				t.Fatalf("segments = %v, want %v", layer["segments"], tt.segments)
			}
			if layer["visible"] != true || layer["tab"] != "segments" {
				t.Fatal("compact layer should open on its segment list")
			}
			selected := state["selectedLayer"].(map[string]interface{})
			if selected["layer"] != layer["name"] || selected["visible"] != true {
				t.Fatal("compact history panel should be open")
			}
			source := layer["source"].(map[string]interface{})
			if source["enableDefaultSubsources"] != false || !reflect.DeepEqual(source["subsources"], map[string]interface{}{"mesh": true}) {
				t.Fatal("compact history must only load meshes, without graph remapping or 2D segmentation")
			}
			_, stageLayers, stageInfo, err := buildCaveHistoryState(results, tt.limit, false)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(info, stageInfo) {
				t.Fatal("compact view lost history labels")
			}
			colors := layer["segmentColors"].(map[string]interface{})
			if len(colors) != len(tt.segments) {
				t.Fatal("hidden roots should also retain their colors")
			}
			for _, stage := range stageLayers {
				for id, color := range stage["segmentColors"].(map[string]interface{}) {
					if colors[id] != color {
						t.Errorf("root %s changed color in compact view", id)
					}
				}
			}
			if err := nglstate.EnsureSegmentPropertiesSource(layer, "https://example.org/history/|neuroglancer-precomputed:", nil); err != nil {
				t.Fatal(err)
			}
			if sources := layer["source"].([]interface{}); len(sources) != 2 || !reflect.DeepEqual(sources[0], source) {
				t.Fatal("attaching labels changed the mesh source settings")
			}
			url, err := nglstate.EncodeURL(state, "")
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := nglstate.DecodeURL(url)
			if err != nil {
				t.Fatal(err)
			}
			wantJSON, _ := json.Marshal(state)
			gotJSON, _ := json.Marshal(decoded)
			if !bytes.Equal(wantJSON, gotJSON) {
				t.Fatal("compact state did not round-trip through viewer URL")
			}
		})
	}
}

func TestCompactCaveHistoryStateMissingBeforeRoots(t *testing.T) {
	results := []caveHistoryResult{{RootID: "100", Entries: []caveHistoryEntry{
		{OperationID: 2, Timestamp: 2, AfterRootIDs: []string{"100", "200"}},
		{OperationID: 1, Timestamp: 1, BeforeRootIDs: []string{"50"}, AfterRootIDs: []string{"100"}},
	}}}
	_, layers, _, err := buildCaveHistoryState(results, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []interface{}{"!50", "100", "200"}; !reflect.DeepEqual(layers[0]["segments"], want) {
		t.Fatalf("segments = %v, want latest after roots visible: %v", layers[0]["segments"], want)
	}
}

func TestCaveHistoryCompactFlagDefaultsToTrue(t *testing.T) {
	flag := caveHistoryCmd.Flags().Lookup("compact")
	if flag == nil || flag.DefValue != "true" {
		t.Fatal("cave-history should default to compact mode")
	}
}

func TestCaveHistoryStateTimestampTieAndMissingBeforeRoots(t *testing.T) {
	results := []caveHistoryResult{{RootID: "100", Entries: caveRowsToHistoryEntries([]cave.ChangeLogRow{
		{OperationID: 1, Timestamp: 1700000000000, BeforeRootIDs: []uint64{7}, AfterRootIDs: []uint64{8}},
		{OperationID: 2, Timestamp: 1700000000000, AfterRootIDs: []uint64{20, 3, 20}, UserID: 42},
	})}}
	_, layers, info, err := buildCaveHistoryState(results, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 {
		t.Fatalf("layers = %d, want queried root and latest after stage", len(layers))
	}
	if layers[0]["visible"] != false || layers[1]["visible"] != true {
		t.Fatal("latest after stage should be visible when before roots are absent")
	}
	if want := "Root 100 | split #2 | user 42 | 2023-11-14T22:13:20Z | after (reported)"; layers[1]["name"] != want {
		t.Fatalf("layer name = %v, want %q", layers[1]["name"], want)
	}
	var props struct {
		Inline struct {
			IDs        []string `json:"ids"`
			Properties []struct {
				Values []string `json:"values"`
			} `json:"properties"`
		} `json:"inline"`
	}
	if err := json.Unmarshal(info, &props); err != nil {
		t.Fatal(err)
	}
	if want := []string{"3", "20", "100"}; !reflect.DeepEqual(props.Inline.IDs, want) {
		t.Fatalf("label IDs = %v, want %v", props.Inline.IDs, want)
	}
	label := "after (reported) split #2 | user 42 | 2023-11-14T22:13:20Z"
	if want := []string{label, label, "Queried root 100"}; !reflect.DeepEqual(props.Inline.Properties[0].Values, want) {
		t.Fatalf("labels = %v, want %v", props.Inline.Properties[0].Values, want)
	}
	if results[0].Entries[0].OperationID != 1 {
		t.Fatal("builder mutated input ordering")
	}
}

func TestCaveHistoryStateWithoutResults(t *testing.T) {
	state, layers, info, err := buildCaveHistoryState(nil, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 0 || len(state["layers"].([]interface{})) != 1 {
		t.Fatal("empty history should retain only the image layer")
	}
	if _, ok := state["selectedLayer"]; ok {
		t.Fatal("empty history retained a selected layer from the template")
	}
	if !bytes.Contains(info, []byte(`"ids":[]`)) || !bytes.Contains(info, []byte(`"values":[]`)) {
		t.Fatalf("empty history labels = %s, want empty arrays", info)
	}
}

func TestRunCaveHistoryViewerDelivery(t *testing.T) {
	originalDeliver, originalLabels := caveHistoryDeliverState, caveHistoryAttachLabels
	t.Cleanup(func() { caveHistoryDeliverState, caveHistoryAttachLabels = originalDeliver, originalLabels })
	t.Setenv("CRANT_LABELS_HOOK", "configured-hook")
	for _, labelFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "label failure"}[labelFailure], func(t *testing.T) {
			published, delivered := false, false
			caveHistoryAttachLabels = func(_ io.Writer, layers []map[string]interface{}, info []byte, _ string, ttl time.Duration, hook string) error {
				published = true
				if len(layers) != 1 || !bytes.Contains(info, []byte("Ada")) || hook != "configured-hook" || ttl != time.Hour {
					t.Fatalf("unexpected label publication: %d layers, hook %q", len(layers), hook)
				}
				if labelFailure {
					return errors.New("publish failed")
				}
				for _, layer := range layers {
					if err := nglstate.EnsureSegmentPropertiesSource(layer, "https://example.org/labels/|neuroglancer-precomputed:", nil); err != nil {
						t.Fatal(err)
					}
				}
				return nil
			}
			caveHistoryDeliverState = func(result *nglstate.LoadResult, opts nglstate.DeliveryOptions) error {
				delivered = true
				if !opts.Open || opts.OutputFile != "history.json" || result.Source != nglstate.SourceTemplate {
					t.Fatalf("delivery = %#v", opts)
				}
				for _, raw := range result.State["layers"].([]interface{})[1:] {
					if _, ok := raw.(map[string]interface{})["source"].([]interface{}); !ok {
						t.Fatal("delivered layer missing labels")
					}
				}
				return nil
			}
			client := &fakeCaveHistoryClient{rows: map[uint64][]cave.ChangeLogRow{111: {{OperationID: 1, UserName: "Ada", BeforeRootIDs: []uint64{10, 11}, AfterRootIDs: []uint64{111}, IsMerge: true}}}}
			var out, errOut bytes.Buffer
			err := runCaveHistory(&out, &errOut, client, []string{"111"}, caveHistoryOptions{Open: true, Output: "history.json", Compact: true, Labels: true, LabelsTTL: time.Hour})
			if (err != nil) != labelFailure {
				t.Fatalf("error = %v", err)
			}
			if !published || delivered == labelFailure {
				t.Fatalf("published=%v delivered=%v", published, delivered)
			}
			if out.Len() != 0 {
				t.Fatal("table mixed into state output")
			}
		})
	}
}

func TestCaveHistoryViewerOptionValidation(t *testing.T) {
	for _, opts := range []caveHistoryOptions{
		{JSON: true, Open: true}, {JSON: true, Output: "x.json"}, {Labels: true}, {HistoryLimit: -1}, {Open: true, Labels: true, LabelsTTL: -time.Hour},
	} {
		client := &fakeCaveHistoryClient{}
		err := runCaveHistory(io.Discard, io.Discard, client, []string{"111"}, opts)
		if err == nil || len(client.calls) != 0 {
			t.Fatalf("invalid options %+v: error=%v calls=%v", opts, err, client.calls)
		}
	}
}

func TestRunCaveHistorySavesStateWithoutLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	client := &fakeCaveHistoryClient{rows: map[uint64][]cave.ChangeLogRow{
		576460752688642351: {{
			OperationID: 1, Timestamp: 1700000000000, UserName: "Ada", IsMerge: true,
			BeforeRootIDs: []uint64{576460752688642349, 576460752688642350},
			AfterRootIDs:  []uint64{576460752688642351},
		}},
	}}
	var out, errOut bytes.Buffer
	if err := runCaveHistory(&out, &errOut, client, []string{"576460752688642351"}, caveHistoryOptions{Output: path, Filtered: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	layers := state["layers"].([]interface{})
	if len(layers) != 4 {
		t.Fatalf("saved %d layers, want image, queried root, before, after", len(layers))
	}
	before := layers[2].(map[string]interface{})
	if !reflect.DeepEqual(before["segments"], []interface{}{"576460752688642349", "576460752688642350"}) {
		t.Fatalf("historical root IDs lost precision: %v", before["segments"])
	}
	if !strings.Contains(before["name"].(string), "Ada | 2023-11-14T22:13:20Z") {
		t.Fatal("unlabeled scene lacks edit context")
	}
	if out.Len() != 0 {
		t.Fatal("saving state also printed a table")
	}
	// An unwritable output must report an error rather than claim success.
	if err := runCaveHistory(io.Discard, io.Discard, client, []string{"576460752688642351"}, caveHistoryOptions{Output: t.TempDir()}); err == nil {
		t.Fatal("writing a state to a directory unexpectedly succeeded")
	}
}
