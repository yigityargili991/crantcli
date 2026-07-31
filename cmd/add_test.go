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
	t.Run("valid field", func(t *testing.T) {
		got, err := resolveAddColorBy("cell_type", false)
		if err != nil {
			t.Fatalf("resolveAddColorBy returned error: %v", err)
		}
		if got != "cell_type" {
			t.Fatalf("resolveAddColorBy = %q, want cell_type", got)
		}
	})

	t.Run("color-sub validates without color-by grouping", func(t *testing.T) {
		got, err := resolveAddColorBy("", true)
		if err != nil {
			t.Fatalf("resolveAddColorBy returned error: %v", err)
		}
		if got != "" {
			t.Fatalf("resolveAddColorBy = %q, want empty color-by field", got)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		_, err := resolveAddColorBy("cell_type", true)
		if err == nil {
			t.Fatal("expected conflict error")
		}
	})

	t.Run("invalid field", func(t *testing.T) {
		_, err := resolveAddColorBy("not_a_field", false)
		if err == nil {
			t.Fatal("expected invalid field error")
		}
	})
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

	applyAddSegmentColors(layer, []string{"a1", "a2", "b1", "b2"}, groups, subtypeMap, "colored", "", true)

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

	applyAddSegmentColors(layer, []string{"a1", "a2", "b1", "b2"}, groups, nil, "colored", "column", false)

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
