package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"crantcli/internal/clipboard"
	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"
)

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
		colorSub, labels                 bool
		labelsHook                       string
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
		addColorSub,
		addLabels,
		addLabelsHook,
	}
	previousNewClient := addNewClient
	previousQuery := addQueryNeurons
	previousLoad := addLoadState
	previousFind := addFindSegmentationLayer
	previousDeliver := addDeliverState
	previousClipboard := addClipboardWrite
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
		addColorSub = previousOptions.colorSub
		addLabels = previousOptions.labels
		addLabelsHook = previousOptions.labelsHook
		addNewClient = previousNewClient
		addQueryNeurons = previousQuery
		addLoadState = previousLoad
		addFindSegmentationLayer = previousFind
		addDeliverState = previousDeliver
		addClipboardWrite = previousClipboard
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
	addColorSub = false
	addLabels = false
	addLabelsHook = ""

	addNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
	addQueryNeurons = func(*seatable.Client, *seatable.Filters) ([]seatable.NeuronRow, error) {
		return []seatable.NeuronRow{{RootID: "100"}}, nil
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
	return layer
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
		colorSub  bool
		want      []string
		wantError string
	}{
		{name: "valid field", colorBy: "cell_type", want: []string{"cell_type"}},
		{name: "two fields nest", colorBy: "cell_type,cell_subtype", want: []string{"cell_type", "cell_subtype"}},
		{name: "surrounding space is trimmed", colorBy: " cell_type , cell_subtype ", want: []string{"cell_type", "cell_subtype"}},
		{name: "color-sub validates without color-by grouping", colorSub: true},
		{
			name:      "conflict",
			colorBy:   "cell_type",
			colorSub:  true,
			wantError: "--color-by and --color-sub cannot be used together",
		},
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
			got, err := resolveAddColorBy(tt.colorBy, tt.colorSub)
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

// TestAddColorSubIsDeprecated pins the first stage of retiring --color-sub: the
// flag keeps working, but it warns and no longer appears in help or the
// generated command docs. The warning names the release that removes it, so a
// caller reading it learns the deadline and not only the replacement; the same
// version is promised in docs/guides/color.md. The advice has to name a color
// mode, since two-level
// coloring falls back to the inner field alone under anything but colored --
// advising group,cell_subtype on its own would be contradicted by that warning
// one line later. The rejected phrasings are the ones that claimed more than
// they delivered: query groups are not always cell_type values, and "matches
// one metadata field" also describes a mixed union over several fields.
func TestAddColorSubIsDeprecated(t *testing.T) {
	flag := addCmd.Flags().Lookup("color-sub")
	if flag == nil {
		t.Fatal("--color-sub flag is missing")
	}
	if flag.Deprecated == "" {
		t.Fatal("--color-sub carries no deprecation message")
	}
	wants := []string{
		"removed in v0.19.0",
		"--color-by group,cell_subtype with --color colored",
		"--color-by cell_subtype with a named family",
	}
	for _, want := range wants {
		if !strings.Contains(flag.Deprecated, want) {
			t.Fatalf("deprecation message = %q, want it to contain %q", flag.Deprecated, want)
		}
	}
	rejects := map[string]string{
		"--color-by cell_type,cell_subtype": "assumes every query group is a cell_type",
		"matches one metadata field":        "also describes a mixed union over several fields",
		"for one query group":               "promises an equivalence that only holds under a named family",
		"keep --color-sub":                  "--color-by group,cell_subtype now reproduces every query group",
	}
	for reject, why := range rejects {
		if strings.Contains(flag.Deprecated, reject) {
			t.Fatalf("deprecation message %q: %s", flag.Deprecated, why)
		}
	}
	if !flag.Hidden {
		t.Fatal("a deprecated --color-sub should be hidden from help output")
	}
	if usage := addCmd.Flags().FlagUsages(); strings.Contains(usage, "--color-sub") {
		t.Fatalf("flag usage still advertises --color-sub:\n%s", usage)
	}
}

func TestValidateAddOptions(t *testing.T) {
	tests := []struct {
		name      string
		regions   []string
		bundles   []string
		color     string
		colorBy   string
		colorSub  bool
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
			name:      "color by conflicts with color sub",
			bundles:   []string{"LX"},
			colorBy:   "region",
			colorSub:  true,
			wantError: "--color-by and --color-sub cannot be used together",
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
			err := validateAddOptions(tt.regions, tt.bundles, tt.color, tt.colorBy, tt.colorSub)
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

func TestApplyAddSegmentColors_ColorSubKeepsSubtypeWithinQueryGroups(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{
		{"a1", "a2"},
		{"b1", "b2"},
	}
	subtypeMap := map[string]string{
		"a1": "shared",
		"a2": "",
		"b1": "shared",
		"b2": "other",
	}

	applyAddSegmentColors(layer, addColorPlan{
		rootIDs:    []string{"a1", "a2", "b1", "b2"},
		groups:     groups,
		subtypeMap: subtypeMap,
		color:      "colored",
		colorSub:   true,
	})

	colors, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %#v", layer["segmentColors"])
	}
	if colors["a1"] == colors["b1"] {
		t.Fatalf("same subtype in different query groups got the same color: a1=%v b1=%v", colors["a1"], colors["b1"])
	}
	if _, ok := colors["a2"]; !ok {
		t.Fatalf("empty subtype should keep its base group color")
	}
	if colors["a2"] == colors["a1"] {
		t.Fatalf("empty subtype should not be recolored as the non-empty subtype in its group")
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

// TestNestColorByPartitions_QueryGroupsSplitByField is the case that lets
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
	if runtime.GOOS == "windows" {
		t.Skip("test hook is a POSIX shell script")
	}

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

	script := filepath.Join(t.TempDir(), "labels-hook.sh")
	hook := `#!/bin/sh
set -eu
case "$1" in
  publish)
    cat >/dev/null
    printf '%s\n' '{"url":"` + newURL + `","id":"new"}'
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

	layer := map[string]interface{}{
		"source": []interface{}{
			"graphene://x",
			map[string]interface{}{"url": oldURL},
		},
	}
	rows := []seatable.NeuronRow{{RootID: "1", CellType: "ER"}}

	if err := attachCellTypeLabels(layer, rows, time.Hour, "sh "+script); err != nil {
		t.Fatal(err)
	}

	want := []interface{}{"graphene://x", map[string]interface{}{"url": newURL}}
	if got := layer["source"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("source = %#v, want %#v", got, want)
	}
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

// TestExtractRootIDsWithSubtype_AllEmpty verifies the same for extractRootIDsWithSubtype.
func TestExtractRootIDsWithSubtype_AllEmpty(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "", CellSubtype: "alpha"},
	}
	ids, sm := extractRootIDsWithSubtype(rows)
	if len(ids) != 0 {
		t.Errorf("expected empty ids, got %v", ids)
	}
	if len(sm) != 0 {
		t.Errorf("expected empty subtypeMap, got %v", sm)
	}
}

// TestExtractRootIDsWithSubtype_NilRows verifies no panic on nil input.
func TestExtractRootIDsWithSubtype_NilRows(t *testing.T) {
	ids, sm := extractRootIDsWithSubtype(nil)
	if len(ids) != 0 {
		t.Errorf("expected empty ids for nil rows, got %v", ids)
	}
	if len(sm) != 0 {
		t.Errorf("expected empty map for nil rows, got %v", sm)
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

func TestExtractRootIDsWithSubtype(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "100", CellSubtype: "alpha"},
		{RootID: "200", CellSubtype: "beta"},
		{RootID: "", CellSubtype: "gamma"}, // no root ID, should be skipped
		{RootID: "300", CellSubtype: ""},   // no subtype
	}

	ids, sm := extractRootIDsWithSubtype(rows)

	wantIDs := []string{"100", "200", "300"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("ids = %v, want %v", ids, wantIDs)
	}

	if sm["100"] != "alpha" {
		t.Errorf("sm[100] = %q, want alpha", sm["100"])
	}
	if sm["200"] != "beta" {
		t.Errorf("sm[200] = %q, want beta", sm["200"])
	}
	if sm["300"] != "" {
		t.Errorf("sm[300] = %q, want empty", sm["300"])
	}
	if _, ok := sm[""]; ok {
		t.Error("empty root ID should not be in subtypeMap")
	}
}
