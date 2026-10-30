// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"crantcli/internal/clipboard"
	"crantcli/internal/labelhost"
	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"
	"crantcli/internal/segprops"
)

// The Long text must match the documented output rules: only a URL/template
// input copies a URL back to the clipboard by default, a state file or piped
// JSON prints to stdout, and --clipboard-ids can still take its base state from
// --state or piped JSON.
func TestAddLongDocumentsOutputDestinations(t *testing.T) {
	for _, want := range []string{
		"write the resulting URL back to the clipboard",
		"a state file or piped JSON prints the formatted state to stdout",
		"--state, piped JSON, or the default template",
	} {
		if !strings.Contains(addCmd.Long, want) {
			t.Errorf("add Long lacks %q", want)
		}
	}
	if strings.Contains(addCmd.Long, "With no --output, the resulting Neuroglancer URL is written back to the clipboard") {
		t.Error("add Long still claims every no---output run writes the URL back to the clipboard")
	}
	if !strings.Contains(addCmd.Example, "crantcli add --clipboard-ids -s base.json --labels") {
		t.Error("add examples lost the --clipboard-ids -s base.json guidance")
	}
}

func TestDeliverRootIDs(t *testing.T) {
	for _, test := range []struct {
		name       string
		copyResult clipboard.WriteResult
		copyErr    error
		wantStatus string
	}{
		{
			name:       "clipboard success",
			copyResult: clipboard.WriteResult{Backend: clipboard.BackendWLCopy},
			wantStatus: "copied root IDs via wl-copy",
		},
		{
			name:       "clipboard failure is warning",
			copyErr:    errors.New("headless"),
			wantStatus: "clipboard copy failed: headless",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var copied string
			err := deliverRootIDs([]string{"100", "200"}, &stdout, &stderr, func(value string) (clipboard.WriteResult, error) {
				copied = value
				return test.copyResult, test.copyErr
			})
			if err != nil {
				t.Fatal(err)
			}
			if stdout.String() != "100\n200\n" || copied != "100\n200" {
				t.Fatalf("stdout = %q, copied = %q", stdout.String(), copied)
			}
			if !strings.Contains(stderr.String(), test.wantStatus) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.wantStatus)
			}
		})
	}
}

func isolateAddCommandRun(t *testing.T) {
	t.Helper()
	previousOptions := struct {
		superClass                       string
		cellClasses, cellTypes, subtypes []string
		intersect                        bool
		side                             string
		regions, bundles                 []string
		tract, proofread                 string
		state                            string
		generate                         bool
		output, layer, color, colorBy    string
		replace, rootIDsOnly, open       bool
		labels                           bool
		labelsHook                       string
		labelBy, labelTags               string
	}{
		addSuperClass,
		addCellClasses,
		addCellTypes,
		addCellSubtypes,
		addIntersect,
		addSide,
		addRegions,
		addBundles,
		addTract,
		addProofread,
		addState,
		addGenerate,
		addOutput,
		addLayer,
		addColor,
		addColorBy,
		addReplace,
		addRootIDsOnly,
		addOpen,
		addLabels,
		addLabelsHook,
		addLabelBy,
		addLabelTags,
	}
	previousNewClient := addNewClient
	previousQuery := addQueryNeurons
	previousLoad := addLoadState
	previousFind := addFindSegmentationLayer
	previousDeliver := addDeliverState
	previousClipboard := addClipboardWrite
	previousClipboardIDs := addClipboardIDs
	previousClipboardRead := addClipboardRead
	previousQueryRootIDs := addQueryNeuronsByRootIDs
	previousGetAPIToken := getAPIToken
	t.Cleanup(func() {
		addSuperClass = previousOptions.superClass
		addCellClasses = previousOptions.cellClasses
		addCellTypes = previousOptions.cellTypes
		addCellSubtypes = previousOptions.subtypes
		addIntersect = previousOptions.intersect
		addSide = previousOptions.side
		addRegions = previousOptions.regions
		addBundles = previousOptions.bundles
		addTract = previousOptions.tract
		addProofread = previousOptions.proofread
		addState = previousOptions.state
		addGenerate = previousOptions.generate
		addOutput = previousOptions.output
		addLayer = previousOptions.layer
		addColor = previousOptions.color
		addColorBy = previousOptions.colorBy
		addReplace = previousOptions.replace
		addRootIDsOnly = previousOptions.rootIDsOnly
		addOpen = previousOptions.open
		addLabels = previousOptions.labels
		addLabelsHook = previousOptions.labelsHook
		addLabelBy = previousOptions.labelBy
		addLabelTags = previousOptions.labelTags
		addNewClient = previousNewClient
		addQueryNeurons = previousQuery
		addLoadState = previousLoad
		addFindSegmentationLayer = previousFind
		addDeliverState = previousDeliver
		addClipboardWrite = previousClipboard
		addClipboardIDs = previousClipboardIDs
		addClipboardRead = previousClipboardRead
		addQueryNeuronsByRootIDs = previousQueryRootIDs
		getAPIToken = previousGetAPIToken
		addCmd.SetOut(nil)
		addCmd.SetErr(nil)
	})

	addSuperClass = ""
	addCellClasses = nil
	addCellTypes = nil
	addCellSubtypes = nil
	addIntersect = false
	addSide = ""
	addRegions = []string{"LX"}
	addBundles = nil
	addTract = ""
	addProofread = ""
	addState = ""
	addGenerate = false
	addOutput = ""
	addLayer = ""
	addColor = ""
	addColorBy = ""
	addReplace = false
	addOpen = false
	addLabels = false
	addLabelsHook = ""
	addLabelBy = defaultLabelBy
	addLabelTags = defaultLabelTags
	addClipboardIDs = false

	addNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
	addQueryNeurons = func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronRow, error) {
		return []seatable.NeuronRow{{RootID: "100"}}, nil
	}
	addClipboardRead = func() (string, error) {
		t.Error("add read the clipboard without --clipboard-ids")
		return "", errors.New("unexpected clipboard read")
	}
	addQueryNeuronsByRootIDs = func(*seatable.Client, []string) ([]seatable.NeuronRow, error) {
		t.Error("add queried root IDs without --clipboard-ids --labels")
		return nil, errors.New("unexpected root ID query")
	}
	getAPIToken = func() string { return "test-token" }
}

// TestAddCommandTokenRequirement covers the conditional SeaTable token check:
// add only asks for a token when it will actually query the table.
func TestAddCommandTokenRequirement(t *testing.T) {
	t.Run("--clipboard-ids without labels needs no token", func(t *testing.T) {
		run := isolateAddClipboardRun(t)
		addClipboardRead = func() (string, error) { return "100 200", nil }
		getAPIToken = func() string { return "" }

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatalf("tokenless --clipboard-ids run failed: %v", err)
		}
		if !run.delivered {
			t.Fatal("state was not delivered")
		}
	})

	t.Run("--clipboard-ids with labels needs a token", func(t *testing.T) {
		isolateAddClipboardRun(t)
		addLabels = true
		getAPIToken = func() string { return "" }

		err := addCmd.RunE(addCmd, nil)
		if !errors.Is(err, errNoSeaTableToken) {
			t.Fatalf("error = %v, want the canonical missing-token error", err)
		}
	})

	t.Run("query mode needs a token", func(t *testing.T) {
		isolateAddCommandRun(t)
		getAPIToken = func() string { return "" }

		err := addCmd.RunE(addCmd, nil)
		if !errors.Is(err, errNoSeaTableToken) {
			t.Fatalf("error = %v, want the canonical missing-token error", err)
		}
	})

	if addCmd.Annotations["requiresToken"] == "true" {
		t.Error("add must not use the static requiresToken annotation; its requirement depends on the flags")
	}
}

func TestAddCommandDeliveryBranches(t *testing.T) {
	t.Run("root IDs only", func(t *testing.T) {
		isolateAddCommandRun(t)
		addRootIDsOnly = true
		var stdout, stderr bytes.Buffer
		addCmd.SetOut(&stdout)
		addCmd.SetErr(&stderr)
		addClipboardWrite = func(value string) (clipboard.WriteResult, error) {
			if value != "100" {
				t.Fatalf("clipboard value = %q", value)
			}
			return clipboard.WriteResult{Backend: clipboard.BackendWLCopy}, nil
		}

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}
		if stdout.String() != "100\n" {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})

	t.Run("state delivery", func(t *testing.T) {
		isolateAddCommandRun(t)
		addRootIDsOnly = false
		layer := map[string]interface{}{"type": "segmentation"}
		result := &nglstate.LoadResult{
			State:  map[string]interface{}{"layers": []interface{}{layer}},
			Source: nglstate.SourceTemplate,
		}
		addLoadState = func(string, bool) (*nglstate.LoadResult, error) {
			return result, nil
		}
		addFindSegmentationLayer = func(map[string]interface{}, string) (map[string]interface{}, int, error) {
			return layer, 0, nil
		}
		delivered := false
		addDeliverState = func(got *nglstate.LoadResult, options nglstate.DeliveryOptions) error {
			delivered = true
			if got != result || options != (nglstate.DeliveryOptions{}) {
				t.Fatalf("delivery = (%#v, %#v)", got, options)
			}
			return nil
		}

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}
		if !delivered {
			t.Fatal("state was not delivered")
		}
	})
}

func addTestStateDelivery(t *testing.T) map[string]interface{} {
	t.Helper()
	layer := map[string]interface{}{"type": "segmentation"}
	addTestStateDeliveryFor(t, layer)
	return layer
}

// addTestStateDeliveryFor stubs the state load, layer lookup, and delivery of an
// add run around the given layer, for tests that need it to carry more than a
// type -- attaching labels, for one, needs a source to attach them to.
func addTestStateDeliveryFor(t *testing.T, layer map[string]interface{}) {
	t.Helper()
	result := &nglstate.LoadResult{
		State:  map[string]interface{}{"layers": []interface{}{layer}},
		Source: nglstate.SourceTemplate,
	}
	addLoadState = func(string, bool) (*nglstate.LoadResult, error) { return result, nil }
	addFindSegmentationLayer = func(map[string]interface{}, string) (map[string]interface{}, int, error) {
		return layer, 0, nil
	}
	addDeliverState = func(got *nglstate.LoadResult, _ nglstate.DeliveryOptions) error {
		if got != result {
			t.Fatalf("delivered result = %#v, want %#v", got, result)
		}
		return nil
	}
}

func TestAddCommandColorByPipelines(t *testing.T) {
	t.Run("query group nested by subtype", func(t *testing.T) {
		isolateAddCommandRun(t)
		addRegions = nil
		addCellClasses = []string{"LNO"}
		addCellTypes = []string{"PEN"}
		addColorBy = "group,cell_subtype"
		addRootIDsOnly = false
		addQueryNeurons = func(_ *seatable.Client, filters *seatable.Filters) ([]seatable.NeuronRow, error) {
			switch {
			case filters.CellClass == "LNO":
				return []seatable.NeuronRow{
					{RootID: "a1", CellSubtype: "PFNc"},
					{RootID: "a2", CellSubtype: "PFNm3"},
				}, nil
			case filters.CellType == "PEN":
				return []seatable.NeuronRow{{RootID: "b1", CellSubtype: "PFNc"}}, nil
			default:
				t.Fatalf("unexpected filters: %+v", *filters)
				return nil, nil
			}
		}
		layer := addTestStateDelivery(t)

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}
		colors := layer["segmentColors"].(map[string]interface{})
		if colors["a1"] == colors["a2"] {
			t.Fatalf("subtypes in one query group share a tone: %v", colors)
		}
		if colors["a1"] == colors["b1"] {
			t.Fatalf("query groups share a palette family: %v", colors)
		}
	})

	t.Run("position gradient", func(t *testing.T) {
		isolateAddCommandRun(t)
		addColorBy = "pos_z"
		addRootIDsOnly = false
		addQueryNeurons = func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronRow, error) {
			return []seatable.NeuronRow{
				{RootID: "low", Z: 10, PositionSet: true},
				{RootID: "high", Z: 30, PositionSet: true},
				{RootID: "unset"},
			}, nil
		}
		layer := addTestStateDelivery(t)

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}
		colors := layer["segmentColors"].(map[string]interface{})
		if colors["low"] != "#440154" || colors["high"] != "#fde725" {
			t.Fatalf("gradient endpoints = low:%v high:%v", colors["low"], colors["high"])
		}
		if colors["unset"] != "#808080" {
			t.Fatalf("unset position = %v, want #808080", colors["unset"])
		}
	})
}

func TestResolveAddRegionFilters(t *testing.T) {
	t.Run("multiple bundles alias regions", func(t *testing.T) {
		got, err := resolveAddRegionFilters(nil, []string{" RW ", "RX", "RW", ""})
		if err != nil {
			t.Fatalf("resolveAddRegionFilters returned error: %v", err)
		}
		want := []string{"RW", "RX"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("resolveAddRegionFilters = %v, want %v", got, want)
		}
	})

	t.Run("conflicting repeated flags", func(t *testing.T) {
		_, err := resolveAddRegionFilters([]string{"LX"}, []string{"LW"})
		if err == nil {
			t.Fatal("expected conflict error when both region and bundle are set")
		}
	})
}

func TestResolveAddColorBy(t *testing.T) {
	tests := []struct {
		name      string
		colorBy   string
		want      []string
		wantError string
	}{
		{name: "valid field", colorBy: "cell_type", want: []string{"cell_type"}},
		{name: "two fields nest", colorBy: "cell_type,cell_subtype", want: []string{"cell_type", "cell_subtype"}},
		{name: "surrounding space is trimmed", colorBy: " cell_type , cell_subtype ", want: []string{"cell_type", "cell_subtype"}},
		{name: "invalid field", colorBy: "not_a_field", wantError: `invalid --color-by "not_a_field"`},
		{
			name:      "invalid second field",
			colorBy:   "cell_type,not_a_field",
			wantError: `invalid --color-by "not_a_field"`,
		},
		{
			name:      "three fields",
			colorBy:   "cell_type,cell_subtype,side",
			wantError: "at most 2 comma-separated fields",
		},
		{name: "empty field", colorBy: "cell_type,", wantError: "empty field"},
		{name: "repeated field", colorBy: "cell_type,cell_type", wantError: `"cell_type" is repeated`},
		{name: "continuous field alone", colorBy: "pos_z", want: []string{"pos_z"}},
		{
			name:      "continuous field cannot be the tone",
			colorBy:   "cell_type,pos_z",
			wantError: `"pos_z" is continuous`,
		},
		{
			name:      "continuous field cannot be the hue",
			colorBy:   "pos_z,cell_type",
			wantError: `"pos_z" is continuous`,
		},
		{name: "query group as the hue", colorBy: "group,cell_subtype", want: []string{"group", "cell_subtype"}},
		{name: "query group alone", colorBy: "group", want: []string{"group"}},
		{
			name:      "query group cannot be the tone",
			colorBy:   "cell_type,group",
			wantError: `"group" names the query a neuron matched`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAddColorBy(tt.colorBy)
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAddColorBy returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolveAddColorBy = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestAddColorSubIsRemoved completes the retirement v0.18.0 began. The flag is
// gone rather than hidden, so a script still passing it fails loudly instead of
// being silently ignored -- the one outcome a deprecation cycle exists to
// prevent. The replacement the warning named for two releases still parses, and
// docs/guides/color.md keeps the migration now that the binary cannot carry it.
func TestAddColorSubIsRemoved(t *testing.T) {
	if flag := addCmd.Flags().Lookup("color-sub"); flag != nil {
		t.Fatal("--color-sub should be removed, not hidden or deprecated")
	}
	if err := addCmd.Flags().Parse([]string{"--color-sub"}); err == nil {
		t.Fatal("--color-sub should be rejected as an unknown flag")
	}
	if _, err := resolveAddColorBy("group,cell_subtype"); err != nil {
		t.Fatalf("the documented replacement --color-by group,cell_subtype no longer parses: %v", err)
	}
}

func TestValidateAddOptions(t *testing.T) {
	tests := []struct {
		name      string
		regions   []string
		bundles   []string
		color     string
		colorBy   string
		wantError string
	}{
		{
			name:      "region bundle conflict",
			regions:   []string{"CX"},
			bundles:   []string{"LX"},
			wantError: "--region and --bundle cannot be used together",
		},
		{
			name:      "invalid color by",
			bundles:   []string{"LX"},
			colorBy:   "not_a_field",
			wantError: `invalid --color-by "not_a_field"`,
		},
		{
			name:      "invalid color",
			bundles:   []string{"LX"},
			color:     "#bad",
			wantError: "invalid color",
		},
		{
			name:    "valid column color by",
			regions: []string{"CX"},
			colorBy: "column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddOptions(tt.regions, tt.bundles, tt.color, tt.colorBy)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateAddOptions returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestValidateAddInputs(t *testing.T) {
	err := validateAddInputs(&seatable.Filters{Region: "LX"}, false)
	if err != nil {
		t.Fatalf("validateAddInputs returned error: %v", err)
	}

	err = validateAddInputs(&seatable.Filters{}, false)
	if err == nil {
		t.Fatal("expected missing-filters error")
	}
}

func TestAddClassifierValidation(t *testing.T) {
	tests := []struct {
		name            string
		apply           func()
		wantErr         bool
		wantCellType    string
		wantCellSubtype string
	}{
		// An empty value still counts as a group flag, but its query group has
		// no WHERE condition and would match every neuron.
		{name: "empty cell class", apply: func() { addCellClasses = []string{""} }, wantErr: true},
		{name: "empty cell type", apply: func() { addCellTypes = []string{""} }, wantErr: true},
		{name: "empty cell subtype", apply: func() { addCellSubtypes = []string{""} }, wantErr: true},
		{name: "mixed empty and valid values", apply: func() { addCellTypes = []string{"", "ER"} }, wantErr: true},
		{
			name: "empty classifier with base filter",
			apply: func() {
				addSide = "left"
				addCellClasses = []string{""}
			},
			wantErr: true,
		},
		{name: "valid classifier", apply: func() { addCellTypes = []string{"ER"} }, wantCellType: "ER"},
		{name: "whitespace classifier keeps its SQL predicate", apply: func() { addCellSubtypes = []string{" "} }, wantCellSubtype: " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateAddCommandRun(t)
			// The helper's default region would make the base filters
			// non-empty on its own.
			addRegions = nil
			addBundles = nil
			addRootIDsOnly = true
			tt.apply()

			clientCreated := false
			addNewClient = func() (*seatable.Client, error) {
				clientCreated = true
				return &seatable.Client{}, nil
			}
			var got *seatable.Filters
			addQueryNeurons = func(_ *seatable.Client, filters *seatable.Filters) ([]seatable.NeuronRow, error) {
				copied := *filters
				got = &copied
				return nil, nil
			}

			err := addCmd.RunE(addCmd, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("the run accepted an empty classifier value")
				}
				if clientCreated {
					t.Error("the run created a SeaTable client before rejecting the empty classifier")
				}
				if got != nil {
					t.Fatalf("the run queried SeaTable with filters %+v", *got)
				}
				return
			}

			if err != nil {
				t.Fatalf("the run rejected a valid classifier: %v", err)
			}
			if got == nil {
				t.Fatal("the run did not query SeaTable")
			}
			if got.CellType != tt.wantCellType || got.CellSubtype != tt.wantCellSubtype {
				t.Fatalf("query filters = %+v, want cell_type %q and cell_subtype %q", *got, tt.wantCellType, tt.wantCellSubtype)
			}
		})
	}
}

func TestBuildColorByGroups(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "100", CellType: "ER", Side: "left"},
		{RootID: "200", CellType: "EPG/PEG", Side: "right"},
		{RootID: "", CellType: "ER", Side: "left"},
		{RootID: "300", CellType: "ER", Side: "right"},
		{RootID: "400", CellType: "", Side: "right"},
	}

	groups, labels := buildColorByGroups(rows, "cell_type")

	wantGroups := [][]string{{"200"}, {"100", "300"}, {"400"}}
	if !reflect.DeepEqual(groups, wantGroups) {
		t.Fatalf("groups = %#v, want %#v", groups, wantGroups)
	}

	wantLabels := []string{"cell_type=EPG/PEG", "cell_type=ER", "cell_type=(empty)"}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %#v, want %#v", labels, wantLabels)
	}
}

func TestBuildColorByGroups_DeterministicOrdering(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "300", CellType: "ER"},
		{RootID: "100", CellType: "ER"},
		{RootID: "400", CellType: ""},
		{RootID: "200", CellType: "EPG/PEG"},
	}

	groups, labels := buildColorByGroups(rows, "cell_type")

	wantGroups := [][]string{{"200"}, {"300", "100"}, {"400"}}
	if !reflect.DeepEqual(groups, wantGroups) {
		t.Fatalf("groups = %#v, want %#v", groups, wantGroups)
	}

	wantLabels := []string{"cell_type=EPG/PEG", "cell_type=ER", "cell_type=(empty)"}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %#v, want %#v", labels, wantLabels)
	}
}

func TestBuildColorByGroupsUsesMatchedRegion(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "100", Region: "CX, LW", MatchedRegions: []string{"LW"}},
		{RootID: "200", Region: "CX, LX", MatchedRegions: []string{"LX"}},
	}

	groups, labels := buildColorByGroups(rows, "region")

	wantLabels := []string{"region=LW", "region=LX"}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %v, want %v", labels, wantLabels)
	}
	wantGroups := [][]string{{"100"}, {"200"}}
	if !reflect.DeepEqual(groups, wantGroups) {
		t.Fatalf("groups = %v, want %v", groups, wantGroups)
	}
}

func TestApplyAddSegmentColors_ColorByUsesOneColorPerGroup(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{
		{"a1", "a2"},
		{"b1", "b2"},
	}

	applyAddSegmentColors(layer, addColorPlan{
		rootIDs:       []string{"a1", "a2", "b1", "b2"},
		groups:        groups,
		color:         "colored",
		colorByFields: []string{"column"},
	})

	colors, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %#v", layer["segmentColors"])
	}
	if colors["a1"] != colors["a2"] {
		t.Fatalf("same color-by group got different colors: a1=%v a2=%v", colors["a1"], colors["a2"])
	}
	if colors["b1"] != colors["b2"] {
		t.Fatalf("same color-by group got different colors: b1=%v b2=%v", colors["b1"], colors["b2"])
	}
	if colors["a1"] == colors["b1"] {
		t.Fatalf("different color-by groups got the same color: %v", colors["a1"])
	}
}

func TestBuildNestedColorByGroups(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "b1", CellType: "PEN", CellSubtype: "ER1"},
		{RootID: "a2", CellType: "ER", CellSubtype: "ER2"},
		{RootID: "a1", CellType: "ER", CellSubtype: "ER1"},
		{RootID: "b2", CellType: "PEN"},
		{RootID: "", CellType: "ER", CellSubtype: "ER1"},
	}

	groups, labels := buildNestedColorByGroups(rows, "cell_type", "cell_subtype")

	wantGroups := [][][]string{
		{{"a1"}, {"a2"}},
		{{"b1"}, {"b2"}},
	}
	if !reflect.DeepEqual(groups, wantGroups) {
		t.Fatalf("groups = %v, want %v", groups, wantGroups)
	}

	wantLabels := [][]string{
		{"cell_type=ER / cell_subtype=ER1", "cell_type=ER / cell_subtype=ER2"},
		{"cell_type=PEN / cell_subtype=ER1", "cell_type=PEN / cell_subtype=(empty)"},
	}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %v, want %v", labels, wantLabels)
	}
}

// TestApplyAddSegmentColors_NestedColorByVariesHueThenTone covers what a
// two-field --color-by has to show that a single field cannot: the outer value
// picks the hue and the inner value the tone within it.
func TestApplyAddSegmentColors_NestedColorByVariesHueThenTone(t *testing.T) {
	layer := map[string]interface{}{}
	nested := [][][]string{
		{{"a1"}, {"a2"}},
		{{"b1"}, {"b2"}},
	}

	applyAddSegmentColors(layer, addColorPlan{
		rootIDs:       []string{"a1", "a2", "b1", "b2"},
		nestedGroups:  nested,
		color:         "colored",
		colorByFields: []string{"cell_type", "cell_subtype"},
	})

	colors, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %#v", layer["segmentColors"])
	}
	if colors["a1"] == colors["a2"] {
		t.Fatalf("inner groups in one family share a tone: a1=%v a2=%v", colors["a1"], colors["a2"])
	}
	if colors["a1"] == colors["b1"] {
		t.Fatalf("the same inner value in different families shares a color: a1=%v b1=%v", colors["a1"], colors["b1"])
	}
	if colors["a2"] == colors["b2"] {
		t.Fatalf("different families share a tone: a2=%v b2=%v", colors["a2"], colors["b2"])
	}
}

func TestFallbackNestedColorByFields(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		color  string
		want   []string
	}{
		{
			name:   "colored keeps both",
			fields: []string{"cell_type", "cell_subtype"},
			color:  "colored",
			want:   []string{"cell_type", "cell_subtype"},
		},
		{
			name:   "named drops outer",
			fields: []string{"cell_type", "cell_subtype"},
			color:  "green",
			want:   []string{"cell_subtype"},
		},
		{
			name:   "hex drops outer",
			fields: []string{"cell_type", "cell_subtype"},
			color:  "#ff0000",
			want:   []string{"cell_subtype"},
		},
		{
			name:   "single field unchanged",
			fields: []string{"cell_subtype"},
			color:  "green",
			want:   []string{"cell_subtype"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fallbackNestedColorByFields(tt.fields, tt.color)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("fallbackNestedColorByFields(%v, %q) = %v, want %v", tt.fields, tt.color, got, tt.want)
			}
		})
	}
}

func TestNamedPaletteTwoFieldColorByMatchesInnerFieldAlone(t *testing.T) {
	// ER1 appears under ER and PEN. Flattening pairs would color a1 and b1
	// differently; coloring by cell_subtype alone must share one tone.
	rows := []seatable.NeuronRow{
		{RootID: "a1", CellType: "ER", CellSubtype: "ER1"},
		{RootID: "a2", CellType: "ER", CellSubtype: "ER2"},
		{RootID: "b1", CellType: "PEN", CellSubtype: "ER1"},
		{RootID: "b2", CellType: "PEN"},
	}

	fields := fallbackNestedColorByFields([]string{"cell_type", "cell_subtype"}, "green")
	groups, _ := buildColorByGroups(rows, fields[0])
	layer := map[string]interface{}{}
	applyAddSegmentColors(layer, addColorPlan{
		groups:        groups,
		color:         "green",
		colorByFields: fields,
	})

	innerGroups, _ := buildColorByGroups(rows, "cell_subtype")
	wantLayer := map[string]interface{}{}
	applyAddSegmentColors(wantLayer, addColorPlan{
		groups:        innerGroups,
		color:         "green",
		colorByFields: []string{"cell_subtype"},
	})

	got, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %#v", layer["segmentColors"])
	}
	want, ok := wantLayer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("inner-field segmentColors missing or wrong type: %#v", wantLayer["segmentColors"])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segmentColors = %v, want inner-field coloring %v", got, want)
	}
	if got["a1"] != got["b1"] {
		t.Fatalf("same inner value in different outer groups must share a tone: a1=%v b1=%v", got["a1"], got["b1"])
	}
}

// TestPartitionRowsByQueryGroup covers what no metadata field can describe: a
// mixed union whose groups come from different columns.
func TestPartitionRowsByQueryGroup(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "a1", CellClass: "LNO", CellSubtype: "PFNc"},
		{RootID: "a2", CellClass: "LNO", CellSubtype: "PFNm3"},
		{RootID: "b1", CellType: "PEN", CellSubtype: "PFNc"},
		{RootID: "", CellClass: "LNO"},
	}
	queryGroups := [][]string{{"a1", "a2"}, {"b1"}, {}}
	queryLabels := []string{"LNO", "PEN", "EPG"}

	partitions := partitionRowsByQueryGroup(rows, queryGroups, queryLabels)

	// The third spec matched nothing, so it takes no palette family.
	if len(partitions) != 2 {
		t.Fatalf("partitions = %d, want 2", len(partitions))
	}
	if partitions[0].label != "group=LNO" || partitions[1].label != "group=PEN" {
		t.Fatalf("labels = %q, %q, want group=LNO, group=PEN", partitions[0].label, partitions[1].label)
	}

	groups, labels := colorByGroupsFromPartitions(partitions)
	if !reflect.DeepEqual(groups, [][]string{{"a1", "a2"}, {"b1"}}) {
		t.Fatalf("groups = %v, want [[a1 a2] [b1]]", groups)
	}
	if !reflect.DeepEqual(labels, []string{"group=LNO", "group=PEN"}) {
		t.Fatalf("labels = %v, want [group=LNO group=PEN]", labels)
	}
}

// TestPartitionRowsByQueryGroup_UnnamedGroupIsNotEmpty covers a query with no
// repeated classifier flags: its single group holds the whole result, so it
// must not be labelled the way a missing field value is.
func TestPartitionRowsByQueryGroup_UnnamedGroupIsNotEmpty(t *testing.T) {
	rows := []seatable.NeuronRow{{RootID: "a1"}, {RootID: "a2"}}

	partitions := partitionRowsByQueryGroup(rows, [][]string{{"a1", "a2"}}, []string{""})

	if len(partitions) != 1 {
		t.Fatalf("partitions = %d, want 1", len(partitions))
	}
	if partitions[0].label != "group=(all)" {
		t.Fatalf("label = %q, want group=(all)", partitions[0].label)
	}
}

// TestNestColorByPartitions_QueryGroupsSplitByField is the case that let
// --color-sub retire: PFNc appears under two query groups drawn from different
// columns, and each keeps its own family.
func TestNestColorByPartitions_QueryGroupsSplitByField(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "a1", CellClass: "LNO", CellSubtype: "PFNc"},
		{RootID: "a2", CellClass: "LNO", CellSubtype: "PFNm3"},
		{RootID: "b1", CellType: "PEN", CellSubtype: "PFNc"},
	}
	partitions := partitionRowsByQueryGroup(rows, [][]string{{"a1", "a2"}, {"b1"}}, []string{"LNO", "PEN"})

	groups, labels := nestColorByPartitions(partitions, "cell_subtype")

	wantGroups := [][][]string{{{"a1"}, {"a2"}}, {{"b1"}}}
	if !reflect.DeepEqual(groups, wantGroups) {
		t.Fatalf("groups = %v, want %v", groups, wantGroups)
	}
	wantLabels := [][]string{
		{"group=LNO / cell_subtype=PFNc", "group=LNO / cell_subtype=PFNm3"},
		{"group=PEN / cell_subtype=PFNc"},
	}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %v, want %v", labels, wantLabels)
	}

	layer := map[string]interface{}{}
	applyAddSegmentColors(layer, addColorPlan{
		rootIDs:       []string{"a1", "a2", "b1"},
		nestedGroups:  groups,
		color:         "colored",
		colorByFields: []string{"group", "cell_subtype"},
	})
	colors := layer["segmentColors"].(map[string]interface{})
	if colors["a1"] == colors["b1"] {
		t.Fatalf("PFNc in different query groups shares a color: a1=%v b1=%v", colors["a1"], colors["b1"])
	}
	if colors["a1"] == colors["a2"] {
		t.Fatalf("different subtypes in one query group share a tone: a1=%v a2=%v", colors["a1"], colors["a2"])
	}
}

// TestReportUncoloredQueryGroups covers the groups --color-by group leaves out:
// dropping one also respaces the palettes of the groups that remain, so it must
// not happen silently.
func TestReportUncoloredQueryGroups(t *testing.T) {
	var report bytes.Buffer

	reportUncoloredQueryGroups(&report, [][]string{{"a1"}, {}, {"c1"}}, []string{"ER", "ER1", "PEN"})

	if want := "  group=ER1: no root IDs of its own; not colored\n"; report.String() != want {
		t.Fatalf("report = %q, want %q", report.String(), want)
	}

	// An unnamed group is the whole result, and it keeps that name whether or
	// not it holds anything.
	var unnamed bytes.Buffer
	reportUncoloredQueryGroups(&unnamed, [][]string{{}, {"b1"}}, []string{"", "PEN"})
	if want := "  group=(all): no root IDs of its own; not colored\n"; unnamed.String() != want {
		t.Fatalf("unnamed report = %q, want %q", unnamed.String(), want)
	}

	// A result where nothing matched has no coloring to be left out of.
	var nothing bytes.Buffer
	reportUncoloredQueryGroups(&nothing, [][]string{{}, {}}, []string{"", "PEN"})
	if nothing.Len() != 0 {
		t.Fatalf("report for an empty result = %q, want nothing", nothing.String())
	}
}

func TestBuildGradientColorByValues(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "a", X: 30, Y: 5, Z: 1, PositionSet: true},
		{RootID: "b", X: 10, Y: 5, Z: 2, PositionSet: true},
		{RootID: "c"},
		{RootID: "", X: 99, PositionSet: true},
	}

	got := buildGradientColorByValues(rows, "pos_x")

	wantValues := map[string]float64{"a": 30, "b": 10}
	if !reflect.DeepEqual(got.values, wantValues) {
		t.Fatalf("values = %v, want %v", got.values, wantValues)
	}
	if !reflect.DeepEqual(got.unset, []string{"c"}) {
		t.Fatalf("unset = %v, want [c]", got.unset)
	}
	if got.low != 10 || got.high != 30 {
		t.Fatalf("range = %g to %g, want 10 to 30", got.low, got.high)
	}
}

// TestApplyAddSegmentColors_GradientSpreadsAlongRamp covers the continuous
// path: one color per neuron along the ramp, and a neutral gray for the
// neurons the field has no value for.
func TestApplyAddSegmentColors_GradientSpreadsAlongRamp(t *testing.T) {
	layer := map[string]interface{}{}
	gradient := gradientColorBy{
		values: map[string]float64{"low": 0, "mid": 5, "high": 10},
		unset:  []string{"none"},
		low:    0,
		high:   10,
	}

	applyAddSegmentColors(layer, addColorPlan{
		rootIDs:       []string{"low", "mid", "high", "none"},
		gradient:      &gradient,
		color:         "colored",
		colorByFields: []string{"pos_z"},
	})

	colors, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %#v", layer["segmentColors"])
	}
	for _, pair := range [][2]string{{"low", "mid"}, {"mid", "high"}, {"low", "high"}} {
		if colors[pair[0]] == colors[pair[1]] {
			t.Fatalf("%s and %s share a color: %v", pair[0], pair[1], colors[pair[0]])
		}
	}
	if colors["none"] != "#808080" {
		t.Fatalf("none = %v, want the unset gray #808080", colors["none"])
	}
}

func TestBuildColorByGroups_RootIDGivesEveryNeuronItsOwnGroup(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "200", CellType: "ER"},
		{RootID: "100", CellType: "ER"},
		{RootID: "", CellType: "ER"},
	}

	groups, labels := buildColorByGroups(rows, "root_id")

	wantGroups := [][]string{{"100"}, {"200"}}
	if !reflect.DeepEqual(groups, wantGroups) {
		t.Fatalf("groups = %v, want %v", groups, wantGroups)
	}
	wantLabels := []string{"root_id=100", "root_id=200"}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %v, want %v", labels, wantLabels)
	}
}

// TestReportColorByGroups_RootIDReportsATotal pins the reason root_id needs its
// own report: one line per group would be one line per neuron.
func TestReportColorByGroups_RootIDReportsATotal(t *testing.T) {
	groups := [][]string{{"a"}, {"b"}, {"c"}}
	labels := []string{"root_id=a", "root_id=b", "root_id=c"}

	var summary bytes.Buffer
	reportColorByGroups(&summary, []string{"root_id"}, groups, labels)
	if want := "  root_id: 3 neurons, one color each\n"; summary.String() != want {
		t.Fatalf("root_id report = %q, want %q", summary.String(), want)
	}

	var perGroup bytes.Buffer
	reportColorByGroups(&perGroup, []string{"cell_type"}, groups, []string{"cell_type=ER", "cell_type=PEN", "cell_type=ExR"})
	if lines := strings.Count(perGroup.String(), "\n"); lines != 3 {
		t.Fatalf("cell_type report = %q, want one line per group", perGroup.String())
	}

	var nested bytes.Buffer
	reportNestedColorByGroups(&nested, []string{"cell_type", "root_id"},
		[][][]string{{{"a"}, {"b"}}, {{"c"}}},
		[][]string{{"cell_type=ER / root_id=a", "cell_type=ER / root_id=b"}, {"cell_type=PEN / root_id=c"}})
	if want := "  cell_type,root_id: 3 neurons, one color each\n"; nested.String() != want {
		t.Fatalf("nested root_id report = %q, want %q", nested.String(), want)
	}
}

func TestAttachCellTypeLabels_RemovesExpiredHookURLPrunedByGC(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldURL := "https://hook.example/old/|neuroglancer-precomputed:"
	newURL := "https://hook.example/new/|neuroglancer-precomputed:"
	manifestDir := filepath.Join(home, ".crantcli")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `[
  {
    "id": "old",
    "url": "` + oldURL + `",
    "kind": "hook",
    "created_at": "2000-01-01T00:00:00Z"
  }
]`
	if err := os.WriteFile(filepath.Join(manifestDir, "label_gists.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	script := labelsHookScript(t, "", newURL)

	layer := map[string]interface{}{
		"source": []interface{}{
			"graphene://x",
			map[string]interface{}{"url": oldURL},
		},
	}
	rows := []seatable.NeuronRow{{RootID: "1", CellType: "ER"}}

	if err := attachCellTypeLabels(layer, rows, segprops.DefaultOptions(), time.Hour, script); err != nil {
		t.Fatal(err)
	}

	want := []interface{}{"graphene://x", map[string]interface{}{"url": newURL}}
	if got := layer["source"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("source = %#v, want %#v", got, want)
	}
}

// TestAttachCellTypeLabels_RemovesSourceDeletedByEarlierClean covers a state
// that outlives its label source: 'labels --clean' deletes the source and stops
// tracking it, but a saved state or clipboard URL still carries it. The next
// --labels run must still recognize the source as crantcli's and replace it,
// rather than keep the dead source next to the new one.
func TestAttachCellTypeLabels_RemovesSourceDeletedByEarlierClean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	oldURL := "https://hook.example/old/|neuroglancer-precomputed:"
	newURL := "https://hook.example/new/|neuroglancer-precomputed:"
	rows := []seatable.NeuronRow{{RootID: "1", CellType: "ER"}}
	layer := map[string]interface{}{"source": []interface{}{"graphene://x"}}

	if err := attachCellTypeLabels(layer, rows, segprops.DefaultOptions(), time.Hour, labelsHookScript(t, "", oldURL)); err != nil {
		t.Fatal(err)
	}
	if deleted, _, err := labelhost.Clean(true, 0, labelsHookScript(t, "", oldURL)); err != nil || deleted != 1 {
		t.Fatalf("Clean deleted %d (err %v), want the published source deleted", deleted, err)
	}
	if err := attachCellTypeLabels(layer, rows, segprops.DefaultOptions(), time.Hour, labelsHookScript(t, "", newURL)); err != nil {
		t.Fatal(err)
	}

	want := []interface{}{"graphene://x", map[string]interface{}{"url": newURL}}
	if got := layer["source"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("source = %#v, want the deleted source replaced: %#v", got, want)
	}
}

// TestAddCommandReplaceDropsDeletedLabelSources covers the --replace half of
// the clean-then-replace flow: once 'labels --clean' has deleted a source, an
// add --replace run drops it from the state it rewrites even without --labels,
// and keeps sources crantcli never published. A plain append changes nothing.
func TestAddCommandReplaceDropsDeletedLabelSources(t *testing.T) {
	deadURL := "https://hook.example/old/|neuroglancer-precomputed:"
	foreignURL := "https://example.org/shared/labels/|neuroglancer-precomputed:"
	staleLayer := func() map[string]interface{} {
		return map[string]interface{}{
			"type": "segmentation",
			"source": []interface{}{
				"graphene://x",
				map[string]interface{}{"url": deadURL},
				map[string]interface{}{"url": foreignURL},
			},
			"segments": []interface{}{"7"},
		}
	}
	publishThenClean := func(t *testing.T) {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		hook := labelsHookScript(t, "", deadURL)
		scratch := map[string]interface{}{"source": "graphene://x"}
		if err := attachCellTypeLabels(scratch, []seatable.NeuronRow{{RootID: "7", CellType: "ER"}}, segprops.DefaultOptions(), time.Hour, hook); err != nil {
			t.Fatal(err)
		}
		if deleted, _, err := labelhost.Clean(true, 0, hook); err != nil || deleted != 1 {
			t.Fatalf("Clean deleted %d (err %v), want the published source deleted", deleted, err)
		}
	}

	t.Run("replace drops the deleted source", func(t *testing.T) {
		isolateAddCommandRun(t)
		publishThenClean(t)
		layer := staleLayer()
		addTestStateDeliveryFor(t, layer)
		addReplace = true
		addRootIDsOnly = false

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}

		want := []interface{}{"graphene://x", map[string]interface{}{"url": foreignURL}}
		if got := layer["source"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("source = %#v, want %#v", got, want)
		}
		if got := layer["segments"]; !reflect.DeepEqual(got, []interface{}{"100"}) {
			t.Fatalf("segments = %#v, want the queried root ID only", got)
		}
	})

	t.Run("replace with labels swaps in the new source", func(t *testing.T) {
		isolateAddCommandRun(t)
		publishThenClean(t)
		newURL := "https://hook.example/new/|neuroglancer-precomputed:"
		layer := staleLayer()
		addTestStateDeliveryFor(t, layer)
		addReplace = true
		addLabels = true
		addLabelsHook = labelsHookScript(t, "", newURL)
		addRootIDsOnly = false

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}

		want := []interface{}{
			"graphene://x",
			map[string]interface{}{"url": foreignURL},
			map[string]interface{}{"url": newURL},
		}
		if got := layer["source"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("source = %#v, want %#v", got, want)
		}
	})

	t.Run("append leaves the layer alone", func(t *testing.T) {
		isolateAddCommandRun(t)
		publishThenClean(t)
		layer := staleLayer()
		addTestStateDeliveryFor(t, layer)
		addRootIDsOnly = false

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}

		if got, want := layer["source"], staleLayer()["source"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("source = %#v, want it unchanged: %#v", got, want)
		}
	})
}

// TestAddCommandReplaceDropsTemplateLabelSource covers the built-in template,
// whose segmentation layer ships a label source for its own segments that no
// manifest tracks. --replace swaps those segments out, so it drops that source
// with them; a plain append keeps both the segments and their labels.
func TestAddCommandReplaceDropsTemplateLabelSource(t *testing.T) {
	templateLayer := func(t *testing.T) (map[string]interface{}, []interface{}) {
		t.Helper()
		var scene map[string]interface{}
		if err := json.Unmarshal(nglstate.DefaultScene, &scene); err != nil {
			t.Fatal(err)
		}
		layer, _, err := nglstate.FindSegmentationLayer(scene, "")
		if err != nil {
			t.Fatal(err)
		}
		sources, ok := layer["source"].([]interface{})
		if !ok || len(sources) < 2 {
			t.Skipf("the built-in template's segmentation layer carries no label source to drop: %#v", layer["source"])
		}
		return layer, append([]interface{}{}, sources...)
	}

	for _, test := range []struct {
		name    string
		replace bool
		want    func(sources []interface{}) []interface{}
	}{
		{
			name:    "replace drops it",
			replace: true,
			want:    func(sources []interface{}) []interface{} { return sources[:1] },
		},
		{
			name: "append keeps it",
			want: func(sources []interface{}) []interface{} { return sources },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			isolateAddCommandRun(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			layer, sources := templateLayer(t)
			addTestStateDeliveryFor(t, layer)
			addReplace = test.replace
			addRootIDsOnly = false

			if err := addCmd.RunE(addCmd, nil); err != nil {
				t.Fatal(err)
			}

			if got, want := layer["source"], test.want(sources); !reflect.DeepEqual(got, want) {
				t.Fatalf("source = %#v, want %#v", got, want)
			}
		})
	}
}

// labelsHookScript writes a --labels-hook script that answers publish with url
// and treats clean as a no-op, and returns the command that runs it. The
// published info JSON is captured in publishTo, or discarded when it is empty.
func labelsHookScript(t *testing.T, publishTo, url string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test hook is a POSIX shell script")
	}

	sink := ">/dev/null"
	if publishTo != "" {
		sink = `>"` + publishTo + `"`
	}
	script := filepath.Join(t.TempDir(), "labels-hook.sh")
	hook := `#!/bin/sh
set -eu
case "$1" in
  publish)
    cat ` + sink + `
    printf '%s\n' '{"url":"` + url + `","id":"new"}'
    ;;
  clean)
    exit 0
    ;;
  *)
    exit 2
    ;;
esac
`
	if err := os.WriteFile(script, []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	return "sh " + script
}

// publishedLabelsAndTags reads the label values and the tag vocabulary out of a
// segment-properties info file a labels hook captured.
func publishedLabelsAndTags(t *testing.T, path string) (labels, tags []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Inline struct {
			IDs        []string `json:"ids"`
			Properties []struct {
				Type   string          `json:"type"`
				Values json.RawMessage `json:"values"`
				Tags   []string        `json:"tags"`
			} `json:"properties"`
		} `json:"inline"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("published info is not valid JSON: %v", err)
	}

	for _, property := range info.Inline.Properties {
		switch property.Type {
		case "label":
			if err := json.Unmarshal(property.Values, &labels); err != nil {
				t.Fatal(err)
			}
		case "tags":
			tags = property.Tags
		}
	}
	return labels, tags
}

func TestAttachCellTypeLabels_PublishesChosenFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	published := filepath.Join(t.TempDir(), "info.json")
	script := labelsHookScript(t, published, "https://hook.example/new/|neuroglancer-precomputed:")

	layer := map[string]interface{}{"source": []interface{}{"graphene://x"}}
	rows := []seatable.NeuronRow{
		{RootID: "1", CellType: "PFN", CellSubtype: "PFNc", Side: "left"},
		{RootID: "2", CellType: "PEN", Side: "right"},
	}
	opts, err := resolveLabelOptions("cell_subtype,cell_type", "side")
	if err != nil {
		t.Fatal(err)
	}

	if err := attachCellTypeLabels(layer, rows, opts, time.Hour, script); err != nil {
		t.Fatal(err)
	}

	labels, tags := publishedLabelsAndTags(t, published)

	// cell_subtype labels row 1 and falls back to cell_type for row 2, and only
	// the requested side field becomes a filterable chip.
	if want := []string{"PFNc", "PEN"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
	if want := []string{"side_left", "side_right"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
}

// TestAddCommandPublishesChosenLabelFields covers the wiring the unit tests
// above cannot: that an add run carries its own --label-by and --label-tags all
// the way to what gets published, rather than the built-in configuration.
func TestAddCommandPublishesChosenLabelFields(t *testing.T) {
	isolateAddCommandRun(t)
	t.Setenv("HOME", t.TempDir())

	published := filepath.Join(t.TempDir(), "info.json")
	addLabels = true
	addLabelsHook = labelsHookScript(t, published, "https://hook.example/new/|neuroglancer-precomputed:")
	addLabelBy = "cell_subtype,cell_type"
	addLabelTags = "side"
	addRootIDsOnly = false
	addQueryNeurons = func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronRow, error) {
		return []seatable.NeuronRow{
			{RootID: "1", CellType: "PFN", CellSubtype: "PFNc", Side: "left"},
			{RootID: "2", CellType: "PEN", Side: "right"},
		}, nil
	}
	layer := map[string]interface{}{"type": "segmentation", "source": "graphene://x"}
	addTestStateDeliveryFor(t, layer)

	if err := addCmd.RunE(addCmd, nil); err != nil {
		t.Fatal(err)
	}

	labels, tags := publishedLabelsAndTags(t, published)
	if want := []string{"PFNc", "PEN"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
	if want := []string{"side_left", "side_right"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}

	want := []interface{}{"graphene://x", map[string]interface{}{"url": "https://hook.example/new/|neuroglancer-precomputed:"}}
	if got := layer["source"]; !reflect.DeepEqual(got, want) {
		t.Errorf("source = %#v, want the published labels attached: %#v", got, want)
	}
}

// TestAddCommandRejectsBadLabelFields pins the promise that a mistyped field
// costs nothing: it is caught while cobra validates arguments, before the
// command runs and queries anything.
func TestAddCommandRejectsBadLabelFields(t *testing.T) {
	tests := []struct {
		name      string
		labelBy   string
		labelTags string
		want      string
	}{
		{
			name:      "unknown label field",
			labelBy:   "cell_typo",
			labelTags: defaultLabelTags,
			want:      `invalid --label-by "cell_typo"`,
		},
		{
			name:      "unknown tag field",
			labelBy:   defaultLabelBy,
			labelTags: "cell_class,colour",
			want:      `invalid --label-tags "colour"`,
		},
		{
			name:      "repeated tag field",
			labelBy:   defaultLabelBy,
			labelTags: "side,side",
			want:      `"side" is repeated`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateAddCommandRun(t)
			addLabels = true
			addLabelBy = tt.labelBy
			addLabelTags = tt.labelTags
			addQueryNeurons = func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronRow, error) {
				t.Error("the run queried SeaTable despite an invalid label field")
				return nil, nil
			}

			if err := addCmd.Args(addCmd, nil); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Args error = %v, want it to contain %q", err, tt.want)
			}
			// RunE checks again, for the callers that reach it directly.
			if err := addCmd.RunE(addCmd, nil); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunE error = %v, want it to contain %q", err, tt.want)
			}
		})
	}

	t.Run("known fields pass", func(t *testing.T) {
		isolateAddCommandRun(t)
		addLabelBy = "cell_subtype,cell_type"
		addLabelTags = noLabelTags
		if err := addCmd.Args(addCmd, nil); err != nil {
			t.Fatalf("Args rejected known label fields: %v", err)
		}
	})

	t.Run("query options are still validated", func(t *testing.T) {
		// The label check was added alongside this one; neither may shadow the
		// other.
		isolateAddCommandRun(t)
		addColor = "not-a-color"
		if err := addCmd.Args(addCmd, nil); err == nil {
			t.Fatal("Args accepted an invalid --color")
		}
	})
}

func TestAddColorByFieldValue_AllFields(t *testing.T) {
	row := seatable.NeuronRow{
		SuperClass:   "super",
		CellClass:    "class",
		CellType:     "type",
		CellSubtype:  "subtype",
		CellInstance: "instance",
		Side:         "side",
		Region:       "region",
		Tract:        "tract",
		Nerve:        "nerve",
		Hemilineage:  "hemilineage",
		Proofread:    "proofread",
	}

	tests := []struct {
		field string
		want  string
	}{
		{"super_class", "super"},
		{"cell_class", "class"},
		{"cell_type", "type"},
		{"cell_subtype", "subtype"},
		{"cell_instance", "instance"},
		{"column", "ce"},
		{"side", "side"},
		{"region", "region"},
		{"tract", "tract"},
		{"nerve", "nerve"},
		{"hemilineage", "hemilineage"},
		{"proofread", "proofread"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if got := addColorByFieldValue(row, tt.field); got != tt.want {
				t.Fatalf("addColorByFieldValue(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

func TestColumnFromCellInstance(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		want     string
	}{
		{"regular left", "PFN_L9", "L9"},
		{"regular right", "PFL2_R5", "R5"},
		{"regular compound keeps last pair", "PFL1/3_R_R1R2", "R2"},
		{"delta7", "\u03947_L6R4", "L6R4"},
		{"delta7 longer suffix", "\u03947_L1L10R7", "10R7"},
		{"ascii delta7", "delta7_L6R4", "L6R4"},
		{"short regular", "L", "L"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := columnFromCellInstance(tt.instance); got != tt.want {
				t.Fatalf("columnFromCellInstance(%q) = %q, want %q", tt.instance, got, tt.want)
			}
		})
	}
}

func TestBuildQuerySpecs_CrossProduct(t *testing.T) {
	base := &seatable.Filters{Side: "left"}

	t.Run("both class and type: cross-product", func(t *testing.T) {
		specs := buildQuerySpecs(base, []string{"kenyon_cell"}, []string{"ER", "EPG/PEG"}, nil, false)
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].filters.CellClass != "kenyon_cell" || specs[0].filters.CellType != "ER" {
			t.Errorf("spec[0] = class=%q type=%q, want kenyon_cell/ER", specs[0].filters.CellClass, specs[0].filters.CellType)
		}
		if specs[1].filters.CellClass != "kenyon_cell" || specs[1].filters.CellType != "EPG/PEG" {
			t.Errorf("spec[1] = class=%q type=%q, want kenyon_cell/EPG/PEG", specs[1].filters.CellClass, specs[1].filters.CellType)
		}
		// Base filter preserved
		if specs[0].filters.Side != "left" {
			t.Errorf("base filter Side lost, got %q", specs[0].filters.Side)
		}
	})

	t.Run("multi class x multi type", func(t *testing.T) {
		specs := buildQuerySpecs(base, []string{"A", "B"}, []string{"X", "Y"}, nil, false)
		if len(specs) != 4 {
			t.Fatalf("got %d specs, want 4", len(specs))
		}
		labels := []string{specs[0].label, specs[1].label, specs[2].label, specs[3].label}
		want := []string{"A/X", "A/Y", "B/X", "B/Y"}
		if !reflect.DeepEqual(labels, want) {
			t.Errorf("labels = %v, want %v", labels, want)
		}
	})

	t.Run("only classes: independent groups", func(t *testing.T) {
		specs := buildQuerySpecs(base, []string{"kenyon_cell", "olfactory"}, nil, nil, false)
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].filters.CellClass != "kenyon_cell" || specs[0].filters.CellType != "" {
			t.Errorf("spec[0] = class=%q type=%q, want kenyon_cell/empty", specs[0].filters.CellClass, specs[0].filters.CellType)
		}
		if specs[1].filters.CellClass != "olfactory" || specs[1].filters.CellType != "" {
			t.Errorf("spec[1] = class=%q type=%q, want olfactory/empty", specs[1].filters.CellClass, specs[1].filters.CellType)
		}
	})

	t.Run("only types: independent groups", func(t *testing.T) {
		specs := buildQuerySpecs(base, nil, []string{"ER", "EPG/PEG"}, nil, false)
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].filters.CellType != "ER" || specs[0].filters.CellClass != "" {
			t.Errorf("spec[0] = class=%q type=%q, want empty/ER", specs[0].filters.CellClass, specs[0].filters.CellType)
		}
	})

	t.Run("neither: single group with base", func(t *testing.T) {
		specs := buildQuerySpecs(base, nil, nil, nil, false)
		if len(specs) != 1 {
			t.Fatalf("got %d specs, want 1", len(specs))
		}
		if specs[0].filters.Side != "left" {
			t.Errorf("base filter Side lost, got %q", specs[0].filters.Side)
		}
		if specs[0].label != "" {
			t.Errorf("label should be empty, got %q", specs[0].label)
		}
	})
}

// TestBuildQuerySpecs_BaseFiltersPreservedInCrossProduct verifies that ALL base filter
// fields are preserved in every cross-product spec, not just Side.
func TestBuildQuerySpecs_BaseFiltersPreservedInCrossProduct(t *testing.T) {
	base := &seatable.Filters{
		SuperClass:  "super1",
		CellSubtype: "sub1",
		Side:        "left",
		Region:      "LX",
		Tract:       "tract1",
		Proofread:   "yes",
	}
	specs := buildQuerySpecs(base, []string{"classA", "classB"}, []string{"typeX"}, nil, false)
	if len(specs) != 2 {
		t.Fatalf("got %d specs, want 2", len(specs))
	}
	for _, s := range specs {
		f := s.filters
		if f.SuperClass != "super1" {
			t.Errorf("spec %q: SuperClass=%q, want super1", s.label, f.SuperClass)
		}
		if f.CellSubtype != "sub1" {
			t.Errorf("spec %q: CellSubtype=%q, want sub1", s.label, f.CellSubtype)
		}
		if f.Side != "left" {
			t.Errorf("spec %q: Side=%q, want left", s.label, f.Side)
		}
		if f.Region != "LX" {
			t.Errorf("spec %q: Region=%q, want LX", s.label, f.Region)
		}
		if f.Tract != "tract1" {
			t.Errorf("spec %q: Tract=%q, want tract1", s.label, f.Tract)
		}
		if f.Proofread != "yes" {
			t.Errorf("spec %q: Proofread=%q, want yes", s.label, f.Proofread)
		}
	}
}

// TestBuildQuerySpecs_CrossProductDoesNotMutateBase verifies that buildQuerySpecs
// does not mutate the original base Filters struct.
func TestBuildQuerySpecs_CrossProductDoesNotMutateBase(t *testing.T) {
	base := &seatable.Filters{Side: "right"}
	original := *base

	buildQuerySpecs(base, []string{"classA"}, []string{"typeX", "typeY"}, nil, false)

	if !reflect.DeepEqual(*base, original) {
		t.Errorf("buildQuerySpecs mutated base: got %+v, want %+v", *base, original)
	}
}

// TestBuildQuerySpecs_CrossProductLabels verifies the label format for cross-product.
func TestBuildQuerySpecs_CrossProductLabels(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, []string{"classA"}, []string{"typeX", "typeY"}, nil, false)
	if len(specs) != 2 {
		t.Fatalf("got %d specs, want 2", len(specs))
	}
	if specs[0].label != "classA/typeX" {
		t.Errorf("spec[0].label = %q, want classA/typeX", specs[0].label)
	}
	if specs[1].label != "classA/typeY" {
		t.Errorf("spec[1].label = %q, want classA/typeY", specs[1].label)
	}
}

// TestBuildQuerySpecs_ClassOnlyLabelIsClassName verifies single-dimension (class-only)
// labels match the class name.
func TestBuildQuerySpecs_ClassOnlyLabelIsClassName(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, []string{"kenyon_cell", "olfactory"}, nil, nil, false)
	if specs[0].label != "kenyon_cell" {
		t.Errorf("spec[0].label = %q, want kenyon_cell", specs[0].label)
	}
	if specs[1].label != "olfactory" {
		t.Errorf("spec[1].label = %q, want olfactory", specs[1].label)
	}
}

// TestBuildQuerySpecs_TypeOnlyLabelIsTypeName verifies single-dimension (type-only)
// labels match the type name.
func TestBuildQuerySpecs_TypeOnlyLabelIsTypeName(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, nil, []string{"ER", "EPG/PEG"}, nil, false)
	if specs[0].label != "ER" {
		t.Errorf("spec[0].label = %q, want ER", specs[0].label)
	}
	if specs[1].label != "EPG/PEG" {
		t.Errorf("spec[1].label = %q, want EPG/PEG", specs[1].label)
	}
}

// TestBuildQuerySpecs_EmptySlicesEquivalentToNil verifies that empty slices []string{}
// behave identically to nil for both cellClasses and cellTypes.
func TestBuildQuerySpecs_EmptySlicesEquivalentToNil(t *testing.T) {
	base := &seatable.Filters{Side: "left"}

	specsNil := buildQuerySpecs(base, nil, nil, nil, false)
	specsEmpty := buildQuerySpecs(base, []string{}, []string{}, nil, false)

	if len(specsNil) != len(specsEmpty) {
		t.Fatalf("nil vs empty: got %d vs %d specs", len(specsNil), len(specsEmpty))
	}
	if len(specsNil) != 1 {
		t.Fatalf("expected 1 spec for no-dimensions case, got %d", len(specsNil))
	}
	if specsNil[0].filters.Side != specsEmpty[0].filters.Side {
		t.Errorf("nil vs empty: Side mismatch")
	}
	if specsNil[0].label != "" || specsEmpty[0].label != "" {
		t.Errorf("nil vs empty: expected empty labels, got %q and %q", specsNil[0].label, specsEmpty[0].label)
	}
}

// TestBuildQuerySpecs_CrossProduct_ClassAndTypeNotLeakedToOtherSpecs verifies that
// CellClass assigned to spec[i] is not visible in spec[j] (no shared pointer).
func TestBuildQuerySpecs_CrossProduct_ClassAndTypeNotLeakedToOtherSpecs(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, []string{"A", "B"}, []string{"X", "Y"}, nil, false)
	if len(specs) != 4 {
		t.Fatalf("got %d specs, want 4", len(specs))
	}
	// Mutate one spec's filters and verify others are not affected
	specs[0].filters.Side = "MODIFIED"
	for i := 1; i < len(specs); i++ {
		if specs[i].filters.Side == "MODIFIED" {
			t.Errorf("spec[%d] was affected by mutation of spec[0] — filters are shared (pointer leak)", i)
		}
	}
}

// TestBuildQuerySpecs_SubtypeDimension verifies cell-subtype participates in the
// cross-product like class and type.
func TestBuildQuerySpecs_SubtypeDimension(t *testing.T) {
	base := &seatable.Filters{}

	t.Run("only subtypes: independent groups", func(t *testing.T) {
		specs := buildQuerySpecs(base, nil, nil, []string{"PFNc", "PFNm3"}, false)
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].filters.CellSubtype != "PFNc" || specs[0].label != "PFNc" {
			t.Errorf("spec[0] = subtype=%q label=%q, want PFNc/PFNc", specs[0].filters.CellSubtype, specs[0].label)
		}
		if specs[1].filters.CellSubtype != "PFNm3" || specs[1].label != "PFNm3" {
			t.Errorf("spec[1] = subtype=%q label=%q, want PFNm3/PFNm3", specs[1].filters.CellSubtype, specs[1].label)
		}
	})

	t.Run("class x type x subtype: 3-way cross-product", func(t *testing.T) {
		specs := buildQuerySpecs(base, []string{"A"}, []string{"X"}, []string{"S1", "S2"}, false)
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].label != "A/X/S1" {
			t.Errorf("spec[0].label = %q, want A/X/S1", specs[0].label)
		}
		f := specs[0].filters
		if f.CellClass != "A" || f.CellType != "X" || f.CellSubtype != "S1" {
			t.Errorf("spec[0] filters = class=%q type=%q subtype=%q, want A/X/S1", f.CellClass, f.CellType, f.CellSubtype)
		}
		if specs[1].label != "A/X/S2" {
			t.Errorf("spec[1].label = %q, want A/X/S2", specs[1].label)
		}
	})
}

// TestBuildQuerySpecs_Union verifies that union mode turns every class/type/subtype
// value into its own group (OR across columns) rather than intersecting them.
func TestBuildQuerySpecs_Union(t *testing.T) {
	base := &seatable.Filters{Side: "left"}

	// Mirrors the motivating command: class LNO OR subtypes PFNc/FBt1-NOc/PFNm3 OR type PEN.
	specs := buildQuerySpecs(base,
		[]string{"LNO"},
		[]string{"PEN"},
		[]string{"PFNc", "FBt1-NOc", "PFNm3"},
		true,
	)

	if len(specs) != 5 {
		t.Fatalf("got %d specs, want 5 (one per value)", len(specs))
	}

	type want struct {
		label   string
		class   string
		typ     string
		subtype string
	}
	// Order follows dimension order: classes, then types, then subtypes.
	wants := []want{
		{"LNO", "LNO", "", ""},
		{"PEN", "", "PEN", ""},
		{"PFNc", "", "", "PFNc"},
		{"FBt1-NOc", "", "", "FBt1-NOc"},
		{"PFNm3", "", "", "PFNm3"},
	}
	for i, w := range wants {
		f := specs[i].filters
		if specs[i].label != w.label {
			t.Errorf("spec[%d].label = %q, want %q", i, specs[i].label, w.label)
		}
		if f.CellClass != w.class || f.CellType != w.typ || f.CellSubtype != w.subtype {
			t.Errorf("spec[%d] = class=%q type=%q subtype=%q, want class=%q type=%q subtype=%q",
				i, f.CellClass, f.CellType, f.CellSubtype, w.class, w.typ, w.subtype)
		}
		// Each union group is a single-column filter — no accidental AND across columns.
		nonEmpty := 0
		for _, v := range []string{f.CellClass, f.CellType, f.CellSubtype} {
			if v != "" {
				nonEmpty++
			}
		}
		if nonEmpty != 1 {
			t.Errorf("spec[%d] sets %d classification columns, want exactly 1 (union groups must not intersect columns)", i, nonEmpty)
		}
		// Base scalar filters still apply to every union group.
		if f.Side != "left" {
			t.Errorf("spec[%d]: base filter Side=%q, want left", i, f.Side)
		}
	}
}

// TestBuildQuerySpecs_UnionDoesNotMutateBase guards against pointer/base leakage
// in union mode.
func TestBuildQuerySpecs_UnionDoesNotMutateBase(t *testing.T) {
	base := &seatable.Filters{Side: "right"}
	original := *base

	buildQuerySpecs(base, []string{"A"}, []string{"X"}, []string{"S"}, true)

	if !reflect.DeepEqual(*base, original) {
		t.Errorf("buildQuerySpecs mutated base in union mode: got %+v, want %+v", *base, original)
	}
}

// TestBuildQuerySpecs_UnionVsCrossProduct contrasts the two modes on the same
// mixed-column input: cross-product intersects (one impossible group), union
// spreads into independent groups.
func TestBuildQuerySpecs_UnionVsCrossProduct(t *testing.T) {
	base := &seatable.Filters{}

	cross := buildQuerySpecs(base, []string{"LNO"}, []string{"PEN"}, nil, false)
	if len(cross) != 1 {
		t.Fatalf("cross-product: got %d specs, want 1", len(cross))
	}
	if cross[0].filters.CellClass != "LNO" || cross[0].filters.CellType != "PEN" {
		t.Errorf("cross-product spec should AND class and type, got class=%q type=%q",
			cross[0].filters.CellClass, cross[0].filters.CellType)
	}

	union := buildQuerySpecs(base, []string{"LNO"}, []string{"PEN"}, nil, true)
	if len(union) != 2 {
		t.Fatalf("union: got %d specs, want 2", len(union))
	}
}

// TestGroupDimensionCount checks the threshold that gates the --intersect no-op
// warning: --intersect only matters when two or more dimensions are combined.
func TestGroupDimensionCount(t *testing.T) {
	cases := []struct {
		name                     string
		classes, types, subtypes []string
		want                     int
	}{
		{"none", nil, nil, nil, 0},
		{"one dimension, many values", nil, []string{"ER", "PEN"}, nil, 1},
		{"two dimensions", []string{"LNO"}, []string{"PEN"}, nil, 2},
		{"all three", []string{"LNO"}, []string{"PEN"}, []string{"PFNc"}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupDimensionCount(tc.classes, tc.types, tc.subtypes); got != tc.want {
				t.Errorf("groupDimensionCount = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestDedupeUnionResults verifies overlapping union groups collapse to a unique
// set by root ID: the shared neuron stays only in its first group, allRootIDs is
// unique in first-seen order, and rows dedupe by RootID.
func TestDedupeUnionResults(t *testing.T) {
	// Group 0 (class) and group 1 (a type within it) both contain "20".
	groups := [][]string{
		{"10", "20"},
		{"20", "30"},
	}
	rows := []seatable.NeuronRow{
		{RootID: "10", CellType: "A"},
		{RootID: "20", CellType: "A"},
		{RootID: "20", CellType: "A"}, // duplicate from the overlapping group
		{RootID: "30", CellType: "B"},
	}

	gotGroups, gotIDs, gotRows := dedupeUnionResults(groups, rows)

	wantGroups := [][]string{{"10", "20"}, {"30"}}
	if !reflect.DeepEqual(gotGroups, wantGroups) {
		t.Errorf("groups = %#v, want %#v (shared id must stay in its first group only)", gotGroups, wantGroups)
	}
	wantIDs := []string{"10", "20", "30"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("allRootIDs = %#v, want %#v (unique, first-seen order)", gotIDs, wantIDs)
	}
	if len(gotRows) != 3 {
		t.Fatalf("rows = %d, want 3 unique by RootID", len(gotRows))
	}
}

// TestDedupeUnionResults_KeepsRootlessRows verifies rows without a root ID are
// passed through (they carry no identity to dedupe on) and non-overlapping
// groups are left intact.
func TestDedupeUnionResults_KeepsRootlessRows(t *testing.T) {
	groups := [][]string{{"1"}, {"2"}}
	rows := []seatable.NeuronRow{
		{RootID: "1"},
		{RootID: ""},
		{RootID: ""},
		{RootID: "2"},
	}

	gotGroups, gotIDs, gotRows := dedupeUnionResults(groups, rows)

	if !reflect.DeepEqual(gotGroups, [][]string{{"1"}, {"2"}}) {
		t.Errorf("non-overlapping groups changed: %#v", gotGroups)
	}
	if !reflect.DeepEqual(gotIDs, []string{"1", "2"}) {
		t.Errorf("allRootIDs = %#v, want [1 2]", gotIDs)
	}
	if len(gotRows) != 4 {
		t.Errorf("rows = %d, want 4 (both rootless rows preserved)", len(gotRows))
	}
}

// TestExtractRootIDs_AllEmpty verifies that a slice of rows with no root IDs returns empty.
func TestExtractRootIDs_AllEmpty(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "", CellSubtype: "alpha"},
		{RootID: "", CellSubtype: "beta"},
	}
	ids := extractRootIDs(rows)
	if len(ids) != 0 {
		t.Errorf("expected empty ids, got %v", ids)
	}
}

// TestResolveAddRegionFilters_EdgeCases covers trimming and empty combinations.
func TestResolveAddRegionFilters_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		regions []string
		bundles []string
		want    []string
		wantErr bool
	}{
		{"both empty", nil, nil, []string{}, false},
		{"region only", []string{"CX"}, nil, []string{"CX"}, false},
		{"bundle only", nil, []string{"LX"}, []string{"LX"}, false},
		{"both set", []string{"CX"}, []string{"LX"}, nil, true},
		{"region whitespace only", []string{"  "}, nil, []string{}, false},
		{"bundle whitespace only", nil, []string{"  "}, []string{}, false},
		{"region with spaces trimmed", []string{"  CX  "}, nil, []string{"CX"}, false},
		{"bundle with spaces trimmed", nil, []string{"  LX  "}, []string{"LX"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAddRegionFilters(tt.regions, tt.bundles)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractRootIDs keeps the mixed-row case that extractRootIDsWithSubtype
// used to cover before --color-sub retired: rows without a root ID are dropped
// and the rest keep their query order.
func TestExtractRootIDs(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "100", CellSubtype: "alpha"},
		{RootID: "200", CellSubtype: "beta"},
		{RootID: "", CellSubtype: "gamma"}, // no root ID, should be skipped
		{RootID: "300", CellSubtype: ""},   // no subtype
	}

	ids := extractRootIDs(rows)

	wantIDs := []string{"100", "200", "300"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("ids = %v, want %v", ids, wantIDs)
	}
}

// addClipboardRun records what a --clipboard-ids run did with the state after
// the clipboard was read.
type addClipboardRun struct {
	layer     map[string]interface{}
	result    *nglstate.LoadResult
	loaded    bool
	state     string
	generate  bool
	delivered bool
}

// isolateAddClipboardRun stubs the seams a --clipboard-ids run reaches after
// reading the clipboard: the state load, layer lookup, delivery, and every
// SeaTable path. The SeaTable stubs fail the test when reached, so a run
// without --labels is checked never to query rather than assumed not to.
func isolateAddClipboardRun(t *testing.T, existingSegments ...string) *addClipboardRun {
	t.Helper()
	isolateAddCommandRun(t)
	// The shared isolation defaults to a --region query to keep add runnable
	// without filters; --clipboard-ids rejects that combination.
	addRegions = nil
	setFlagForTest(t, addCmd, "clipboard-ids", "true")

	layer := map[string]interface{}{"type": "segmentation", "source": "graphene://x"}
	if len(existingSegments) > 0 {
		segments := make([]interface{}, 0, len(existingSegments))
		for _, segment := range existingSegments {
			segments = append(segments, segment)
		}
		layer["segments"] = segments
	}
	run := &addClipboardRun{layer: layer}
	run.result = &nglstate.LoadResult{
		State:  map[string]interface{}{"layers": []interface{}{layer}},
		Source: nglstate.SourceTemplate,
	}
	addLoadState = func(state string, generate bool) (*nglstate.LoadResult, error) {
		run.loaded, run.state, run.generate = true, state, generate
		return run.result, nil
	}
	addFindSegmentationLayer = func(map[string]interface{}, string) (map[string]interface{}, int, error) {
		return layer, 0, nil
	}
	addDeliverState = func(got *nglstate.LoadResult, _ nglstate.DeliveryOptions) error {
		run.delivered = got == run.result
		return nil
	}
	addNewClient = func() (*seatable.Client, error) {
		t.Error("--clipboard-ids without --labels built a SeaTable client")
		return nil, errors.New("unexpected SeaTable client")
	}
	addQueryNeurons = func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronRow, error) {
		t.Error("--clipboard-ids ran a SeaTable classifier query")
		return nil, errors.New("unexpected classifier query")
	}
	return run
}

// TestAddClipboardIDsInjectsIntoState covers the path that replaces the
// SeaTable classifier query: the copied IDs are parsed, deduped, and injected
// into the segmentation layer -- appended by default, replaced with --replace --
// while the state load never decodes the clipboard as a state URL.
func TestAddClipboardIDsInjectsIntoState(t *testing.T) {
	t.Run("appends by default", func(t *testing.T) {
		run := isolateAddClipboardRun(t, "9")
		addClipboardRead = func() (string, error) { return "100, 200\n100\t300", nil }
		addState = "base.json"

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}

		want := []interface{}{"9", "100", "200", "300"}
		if got := run.layer["segments"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("segments = %#v, want the clipboard IDs appended: %#v", got, want)
		}
		if !run.delivered {
			t.Fatal("state was not delivered")
		}
		if !run.loaded || run.state != "base.json" || !run.generate {
			t.Fatalf("LoadState(%q, %v), want (\"base.json\", true): the clipboard holds IDs, not a state URL", run.state, run.generate)
		}
	})

	t.Run("replace flag replaces the segment list", func(t *testing.T) {
		run := isolateAddClipboardRun(t, "9")
		addClipboardRead = func() (string, error) { return "100 200", nil }
		addReplace = true

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}
		want := []interface{}{"100", "200"}
		if got := run.layer["segments"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("segments = %#v, want the clipboard IDs replacing the existing list: %#v", got, want)
		}
	})

	t.Run("accepts production-shaped uint64 root IDs", func(t *testing.T) {
		run := isolateAddClipboardRun(t)
		addClipboardRead = func() (string, error) {
			return "720575940610453042, 720575940379280858\n720575940610453042\t18446744073709551615", nil
		}

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}
		want := []interface{}{"720575940610453042", "720575940379280858", "18446744073709551615"}
		if got := run.layer["segments"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("segments = %#v, want the deduped clipboard root IDs: %#v", got, want)
		}
		if !run.delivered {
			t.Fatal("state was not delivered")
		}
	})
}

func TestAddClipboardIDsClipboardFailures(t *testing.T) {
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
			run := isolateAddClipboardRun(t)
			addClipboardRead = func() (string, error) { return tt.clip, tt.clipErr }

			err := addCmd.RunE(addCmd, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantError)
			}
			if run.loaded {
				t.Fatal("no state should be loaded when the clipboard yields no IDs")
			}
		})
	}
}

// TestAddClipboardIDsRejectsNonRootIDPayloads: --clipboard-ids must only inject
// root IDs. A Neuroglancer state URL is the likely payload when add has replaced
// the clipboard with its output URL, and any other token -- a label, scientific
// notation -- would otherwise reach the state as a segment ID.
func TestAddClipboardIDsRejectsNonRootIDPayloads(t *testing.T) {
	tests := []struct {
		name      string
		clip      string
		wantError string
	}{
		{
			name:      "Neuroglancer state URL",
			clip:      `https://spelunker.cave-explorer.org/#!{"layers":[]}`,
			wantError: "clipboard holds a Neuroglancer state URL, not root IDs",
		},
		{
			name:      "non-numeric token",
			clip:      "720575940610453042 PFN",
			wantError: `clipboard entry "PFN" is not a root ID`,
		},
		{
			name:      "scientific notation",
			clip:      "720575940610453042 1e18",
			wantError: `clipboard entry "1e18" is not a root ID`,
		},
		{
			name:      "negative number",
			clip:      "720575940610453042 -100",
			wantError: `clipboard entry "-100" is not a root ID`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := isolateAddClipboardRun(t)
			addClipboardRead = func() (string, error) { return tt.clip, nil }

			err := addCmd.RunE(addCmd, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantError)
			}
			if run.loaded || run.delivered {
				t.Fatal("a clipboard without root IDs should not load or deliver a state")
			}
		})
	}
}

// TestAddClipboardIDsRejectsQueryFlags covers the flags that only shape a
// SeaTable classifier query, now that --clipboard-ids supplies the root IDs
// itself.
func TestAddClipboardIDsRejectsQueryFlags(t *testing.T) {
	tests := []struct {
		name     string
		set      func()
		wantFlag string
	}{
		{name: "super class", set: func() { addSuperClass = "sensory" }, wantFlag: "--super-class"},
		{name: "cell class", set: func() { addCellClasses = []string{"LNO"} }, wantFlag: "--cell-class"},
		{name: "cell type", set: func() { addCellTypes = []string{"ER"} }, wantFlag: "--cell-type"},
		{name: "cell subtype", set: func() { addCellSubtypes = []string{"PFNc"} }, wantFlag: "--cell-subtype"},
		{name: "side", set: func() { addSide = "left" }, wantFlag: "--side"},
		{name: "region", set: func() { addRegions = []string{"LX"} }, wantFlag: "--region"},
		{name: "bundle", set: func() { addBundles = []string{"LX"} }, wantFlag: "--bundle"},
		{name: "tract", set: func() { addTract = "LM" }, wantFlag: "--tract"},
		{name: "proofread", set: func() { addProofread = "true" }, wantFlag: "--proofread"},
		{name: "intersect", set: func() { addIntersect = true }, wantFlag: "--intersect"},
		{name: "color by", set: func() { addColorBy = "cell_type" }, wantFlag: "--color-by"},
		{name: "root IDs only", set: func() { addRootIDsOnly = true }, wantFlag: "--root-ids-only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := isolateAddClipboardRun(t)
			addClipboardRead = func() (string, error) {
				t.Error("a rejected flag combination read the clipboard")
				return "", errors.New("unexpected clipboard read")
			}
			tt.set()

			want := "--clipboard-ids cannot be combined with"
			for _, check := range []struct {
				phase string
				err   error
			}{
				// Args is the only phase that runs for the CLI; the helper is
				// checked directly because it is what Args calls.
				{phase: "Args", err: addCmd.Args(addCmd, nil)},
				{phase: "validateAddClipboardOptions", err: validateAddClipboardOptions()},
			} {
				if check.err == nil || !strings.Contains(check.err.Error(), want) || !strings.Contains(check.err.Error(), tt.wantFlag) {
					t.Fatalf("%s error = %v, want it to name %q and %q", check.phase, check.err, want, tt.wantFlag)
				}
			}
			if run.delivered || run.loaded {
				t.Fatal("a rejected flag combination should not load or deliver a state")
			}
		})
	}

	t.Run("state and delivery flags are allowed", func(t *testing.T) {
		isolateAddClipboardRun(t)
		addState = "base.json"
		addOutput = "out.json"
		addLayer = "seg"
		addColor = "red"
		addReplace = true
		addOpen = true
		if err := addCmd.Args(addCmd, nil); err != nil {
			t.Fatalf("Args rejected flags --clipboard-ids supports: %v", err)
		}
	})
}

// TestAddClipboardIDsLabelsUseMetadata is the clipboard half of the label
// wiring add already covers for queried rows: the copied IDs are looked up
// through seatable.QueryNeuronsByRootIDs and the metadata labels them.
func TestAddClipboardIDsLabelsUseMetadata(t *testing.T) {
	t.Run("fetches metadata for the clipboard IDs", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		run := isolateAddClipboardRun(t)
		addClipboardRead = func() (string, error) { return "100 200", nil }
		addLabels = true
		published := filepath.Join(t.TempDir(), "info.json")
		addLabelsHook = labelsHookScript(t, published, "https://hook.example/new/|neuroglancer-precomputed:")

		var queried []string
		addNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
		addQueryNeuronsByRootIDs = func(_ *seatable.Client, ids []string) ([]seatable.NeuronRow, error) {
			queried = ids
			return []seatable.NeuronRow{
				{RootID: "100", CellType: "PFN"},
				{RootID: "200", CellType: "PEN"},
			}, nil
		}

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatal(err)
		}
		if want := []string{"100", "200"}; !reflect.DeepEqual(queried, want) {
			t.Fatalf("queried %v, want the clipboard root IDs %v", queried, want)
		}
		if labels, _ := publishedLabelsAndTags(t, published); !reflect.DeepEqual(labels, []string{"PFN", "PEN"}) {
			t.Fatalf("labels = %v, want the looked-up cell types", labels)
		}
		if !run.delivered {
			t.Fatal("state was not delivered")
		}
	})

	t.Run("partial metadata still injects and labels", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		run := isolateAddClipboardRun(t)
		addClipboardRead = func() (string, error) { return "100 200", nil }
		addLabels = true
		published := filepath.Join(t.TempDir(), "info.json")
		addLabelsHook = labelsHookScript(t, published, "https://hook.example/new/|neuroglancer-precomputed:")
		addNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
		addQueryNeuronsByRootIDs = func(*seatable.Client, []string) ([]seatable.NeuronRow, error) {
			return []seatable.NeuronRow{{RootID: "100", CellType: "PFN"}}, nil
		}

		if err := addCmd.RunE(addCmd, nil); err != nil {
			t.Fatalf("a partial metadata match should still deliver a state: %v", err)
		}
		if !run.delivered {
			t.Fatal("state was not delivered")
		}
		if labels, _ := publishedLabelsAndTags(t, published); !reflect.DeepEqual(labels, []string{"PFN"}) {
			t.Fatalf("labels = %v, want just the row that matched", labels)
		}
	})

	t.Run("no metadata fails", func(t *testing.T) {
		run := isolateAddClipboardRun(t)
		addClipboardRead = func() (string, error) { return "100 200", nil }
		addLabels = true
		addNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
		addQueryNeuronsByRootIDs = func(*seatable.Client, []string) ([]seatable.NeuronRow, error) {
			return nil, nil
		}

		err := addCmd.RunE(addCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "no CRANT metadata found for clipboard root IDs") {
			t.Fatalf("error = %v, want it to report missing CRANT metadata", err)
		}
		if run.delivered {
			t.Fatal("no state should be delivered when its labels have no metadata")
		}
	})

	t.Run("metadata query failure surfaces", func(t *testing.T) {
		run := isolateAddClipboardRun(t)
		addClipboardRead = func() (string, error) { return "100 200", nil }
		addLabels = true
		addNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
		addQueryNeuronsByRootIDs = func(*seatable.Client, []string) ([]seatable.NeuronRow, error) {
			return nil, errors.New("401 unauthorized")
		}

		err := addCmd.RunE(addCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "querying labels for clipboard root IDs") {
			t.Fatalf("error = %v, want it to carry the SeaTable failure", err)
		}
		if run.delivered {
			t.Fatal("no state should be delivered when the label lookup fails")
		}
	})
}
