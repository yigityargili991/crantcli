package nglstate

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestResolveColor_RawHex(t *testing.T) {
	layer := map[string]interface{}{}
	// ResolveColor expects pre-normalized input (from NormalizeColorInput),
	// so hex values must already have '#' prefix and be lowercase.
	tests := []struct {
		input string
		want  string
	}{
		{"#ff0000", "#ff0000"},
		{"#abcdef", "#abcdef"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ResolveColor(layer, tt.input); got != tt.want {
				t.Errorf("ResolveColor(_, %q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveColor_NamedCycling(t *testing.T) {
	palette := colorPalettes["blue"]

	// First call on empty layer: should get palette[0]
	layer := map[string]interface{}{}
	got := ResolveColor(layer, "blue")
	if got != palette[0] {
		t.Fatalf("first blue: got %q, want %q", got, palette[0])
	}

	// Simulate that first tone was applied
	layer["segmentColors"] = map[string]interface{}{"id1": palette[0]}
	got = ResolveColor(layer, "blue")
	if got != palette[1] {
		t.Fatalf("second blue: got %q, want %q", got, palette[1])
	}

	// Simulate two tones used
	layer["segmentColors"] = map[string]interface{}{"id1": palette[0], "id2": palette[1]}
	got = ResolveColor(layer, "blue")
	if got != palette[2] {
		t.Fatalf("third blue: got %q, want %q", got, palette[2])
	}

	// Simulate all tones used, should cycle back to [0]
	allUsed := map[string]interface{}{}
	for i, hex := range palette {
		allUsed[fmt.Sprintf("id%d", i)] = hex
	}
	layer["segmentColors"] = allUsed
	got = ResolveColor(layer, "blue")
	if got != palette[0] {
		t.Fatalf("cycled blue: got %q, want %q", got, palette[0])
	}
}

func TestNormalizeThenResolve_CaseInsensitive(t *testing.T) {
	// Verify the full normalize-then-resolve pipeline handles case/whitespace.
	layer := map[string]interface{}{}
	palette := colorPalettes["red"]

	for _, input := range []string{"Red", "RED", "  red  "} {
		normalized, err := NormalizeColorInput(input)
		if err != nil {
			t.Fatalf("NormalizeColorInput(%q) unexpected error: %v", input, err)
		}
		got := ResolveColor(layer, normalized)
		if got != palette[0] {
			t.Errorf("ResolveColor(_, NormalizeColorInput(%q)) = %q, want %q", input, got, palette[0])
		}
	}
}

func TestResolveColor_AllFamilies(t *testing.T) {
	layer := map[string]interface{}{}
	for name, palette := range colorPalettes {
		got := ResolveColor(layer, name)
		if got != palette[0] {
			t.Errorf("ResolveColor(_, %q) = %q, want %q", name, got, palette[0])
		}
	}
}

func TestResolveColor_Colored(t *testing.T) {
	layer := map[string]interface{}{}
	// ResolveColor expects pre-normalized "colored" (lowercase).
	got := ResolveColor(layer, "colored")
	if !strings.HasPrefix(got, "#") || len(got) != 7 {
		t.Errorf("ResolveColor(_, \"colored\") = %q, want #RRGGBB format", got)
	}

	// Verify the normalize-then-resolve pipeline handles "Colored" (capitalized).
	normalized, err := NormalizeColorInput("Colored")
	if err != nil {
		t.Fatalf("NormalizeColorInput(\"Colored\") unexpected error: %v", err)
	}
	got2 := ResolveColor(layer, normalized)
	if !strings.HasPrefix(got2, "#") || len(got2) != 7 {
		t.Errorf("ResolveColor(_, NormalizeColorInput(\"Colored\")) = %q, want #RRGGBB format", got2)
	}
}

func TestCountPaletteTones(t *testing.T) {
	palette := colorPalettes["green"]

	// Same tone on multiple IDs counts as 1
	layer := map[string]interface{}{
		"segmentColors": map[string]interface{}{
			"a": palette[0],
			"b": palette[0],
			"c": palette[0],
		},
	}
	if got := countPaletteTones(layer, palette); got != 1 {
		t.Errorf("expected 1 distinct tone, got %d", got)
	}

	// Two different tones
	layer["segmentColors"] = map[string]interface{}{
		"a": palette[0],
		"b": palette[1],
	}
	if got := countPaletteTones(layer, palette); got != 2 {
		t.Errorf("expected 2 distinct tones, got %d", got)
	}

	// Non-palette colors are ignored
	layer["segmentColors"] = map[string]interface{}{
		"a": "#ffffff",
		"b": palette[0],
	}
	if got := countPaletteTones(layer, palette); got != 1 {
		t.Errorf("expected 1 (non-palette ignored), got %d", got)
	}

	// Empty layer
	emptyLayer := map[string]interface{}{}
	if got := countPaletteTones(emptyLayer, palette); got != 0 {
		t.Errorf("expected 0 for empty layer, got %d", got)
	}
}

func TestNormalizeColorInput_NewPalettes(t *testing.T) {
	newPalettes := []string{"orange", "purple", "yellow", "pink", "brown", "indigo", "teal", "lime"}
	for _, name := range newPalettes {
		t.Run(name, func(t *testing.T) {
			got, err := NormalizeColorInput(name)
			if err != nil {
				t.Fatalf("NormalizeColorInput(%q) error: %v", name, err)
			}
			if got != name {
				t.Fatalf("NormalizeColorInput(%q) = %q, want %q", name, got, name)
			}
		})
	}
}

func TestPaletteNamesMatchPalettes(t *testing.T) {
	if len(paletteNames) != len(colorPalettes) {
		t.Errorf("paletteNames has %d entries but colorPalettes has %d", len(paletteNames), len(colorPalettes))
	}
	for _, name := range paletteNames {
		if _, ok := colorPalettes[name]; !ok {
			t.Errorf("paletteNames contains %q but colorPalettes does not", name)
		}
	}
}

// TestSetSegmentColorByGroups_EmptyGroupShiftsPalette verifies that an empty group
// (from a query that returned 0 neurons) still consumes a palette index in "colored" mode,
// meaning subsequent non-empty groups get later palettes rather than the next available one.
func TestSetSegmentColorByGroups_EmptyGroupShiftsPalette(t *testing.T) {
	layer := map[string]interface{}{}
	// With 3 groups, stride=4: indices 0,4,8 → blue, orange, brown.
	// Empty group 1 consumes the orange slot.
	groups := [][]string{
		{"id1"},
		{},
		{"id2"},
	}
	SetSegmentColorByGroups(layer, groups, "colored")

	colors := layer["segmentColors"].(map[string]interface{})
	blue := colorPalettes["blue"]
	brown := colorPalettes["brown"] // index 2*4=8

	if colors["id1"] != blue[0] {
		t.Errorf("id1 (group 0) = %v, want blue[0]=%s", colors["id1"], blue[0])
	}
	if colors["id2"] != brown[0] {
		t.Errorf("id2 (group 2, after empty group 1) = %v, want brown[0]=%s; empty group consumed orange slot", colors["id2"], brown[0])
	}
}

// TestSetSegmentColorByGroups_DuplicateIDsAcrossGroups verifies that when the same ID
// appears in multiple groups, the last group's color wins (last-write semantics).
func TestSetSegmentColorByGroups_DuplicateIDsAcrossGroups(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{
		{"shared", "a_only"},
		{"shared", "b_only"},
	}
	SetSegmentColorByGroups(layer, groups, "colored")

	colors := layer["segmentColors"].(map[string]interface{})
	yellow := colorPalettes["yellow"] // 2 groups stride 6: group 1 → index 6 → yellow

	// "shared" appears at j=0 in both groups. Group 1 writes last -> yellow[0].
	if colors["shared"] != yellow[0] {
		t.Errorf("duplicate id 'shared': got %v, want yellow[0]=%s (last group wins)", colors["shared"], yellow[0])
	}
	// Unique IDs keep their group's color.
	if colors["a_only"] != colorPalettes["blue"][1] {
		t.Errorf("a_only: got %v, want blue[1]=%s", colors["a_only"], colorPalettes["blue"][1])
	}
	if colors["b_only"] != yellow[1] {
		t.Errorf("b_only: got %v, want yellow[1]=%s", colors["b_only"], yellow[1])
	}
}

// TestSetSegmentColorByGroups_MoreThan12Groups verifies that >12 groups wraps palette
// assignment back to the start without panicking.
func TestSetSegmentColorByGroups_MoreThan12Groups(t *testing.T) {
	layer := map[string]interface{}{}
	groups := make([][]string, 13)
	for i := range groups {
		groups[i] = []string{fmt.Sprintf("id_%d", i)}
	}

	// Must not panic
	SetSegmentColorByGroups(layer, groups, "colored")

	colors := layer["segmentColors"].(map[string]interface{})
	// Group 12 wraps to paletteNames[0] = blue; same as group 0
	blue := colorPalettes["blue"]
	if colors["id_0"] != blue[0] {
		t.Errorf("id_0 (group 0) = %v, want blue[0]=%s", colors["id_0"], blue[0])
	}
	if colors["id_12"] != blue[0] {
		t.Errorf("id_12 (group 12, wraps to blue) = %v, want blue[0]=%s", colors["id_12"], blue[0])
	}
}

// TestSetSegmentColorByGroups_NilGroups verifies no panic on nil groups slice.
func TestSetSegmentColorByGroups_NilGroups(t *testing.T) {
	layer := map[string]interface{}{}
	// Must not panic
	SetSegmentColorByGroups(layer, nil, "colored")
	// segmentColors gets set to empty map (function still writes it even with 0 groups)
	// That's fine — just no panics and no colors assigned.
	if colors, ok := layer["segmentColors"].(map[string]interface{}); ok {
		if len(colors) != 0 {
			t.Errorf("nil groups should produce no colors, got %v", colors)
		}
	}
}

// TestSetSegmentColorByGroups_NamedColor_ToneContinuity verifies that with a named
// color, tone indices continue across groups (they don't reset per group).
func TestSetSegmentColorByGroups_NamedColor_ToneContinuity(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{
		{"a1", "a2", "a3"},
		{"b1", "b2"},
	}
	SetSegmentColorByGroups(layer, groups, "blue")

	colors := layer["segmentColors"].(map[string]interface{})
	blue := colorPalettes["blue"]

	// Group 0: tones 0, 1, 2
	if colors["a1"] != blue[0] {
		t.Errorf("a1 = %v, want blue[0]=%s", colors["a1"], blue[0])
	}
	if colors["a2"] != blue[1] {
		t.Errorf("a2 = %v, want blue[1]=%s", colors["a2"], blue[1])
	}
	if colors["a3"] != blue[2] {
		t.Errorf("a3 = %v, want blue[2]=%s", colors["a3"], blue[2])
	}
	// Group 1: tones CONTINUE from 3, 4 (not reset to 0)
	if colors["b1"] != blue[3] {
		t.Errorf("b1 = %v, want blue[3]=%s (tone index continues across groups)", colors["b1"], blue[3])
	}
	if colors["b2"] != blue[4] {
		t.Errorf("b2 = %v, want blue[4]=%s (tone index continues across groups)", colors["b2"], blue[4])
	}
}

func TestSetSegmentColorByGroupValues_ColoredOneColorPerGroup(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{
		{"a1", "a2"},
		{"b1"},
		{"c1", "c2"},
	}

	SetSegmentColorByGroupValues(layer, groups, "colored")

	colors := layer["segmentColors"].(map[string]interface{})
	if colors["a1"] != colors["a2"] {
		t.Fatalf("same group got different colors: a1=%v a2=%v", colors["a1"], colors["a2"])
	}
	if colors["c1"] != colors["c2"] {
		t.Fatalf("same group got different colors: c1=%v c2=%v", colors["c1"], colors["c2"])
	}
	if colors["a1"] == colors["b1"] || colors["a1"] == colors["c1"] || colors["b1"] == colors["c1"] {
		t.Fatalf("different groups should have distinct categorical colors: %v", colors)
	}
}

func TestSetSegmentColorByGroupValues_ColoredManyGroupsUnique(t *testing.T) {
	layer := map[string]interface{}{}
	groups := make([][]string, 40)
	for i := range groups {
		groups[i] = []string{fmt.Sprintf("id_%d", i)}
	}

	SetSegmentColorByGroupValues(layer, groups, "colored")

	colors := layer["segmentColors"].(map[string]interface{})
	seen := make(map[interface{}]bool, len(groups))
	for i := range groups {
		color := colors[fmt.Sprintf("id_%d", i)]
		if seen[color] {
			t.Fatalf("categorical color repeated at group %d: %v", i, color)
		}
		seen[color] = true
	}
}

// TestSetSegmentColorByGroups_HexColor verifies hex color assigns the same value to all IDs.
func TestSetSegmentColorByGroups_HexColor(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{{"a1", "a2"}, {"b1"}}
	SetSegmentColorByGroups(layer, groups, "#aabbcc")

	colors := layer["segmentColors"].(map[string]interface{})
	for _, id := range []string{"a1", "a2", "b1"} {
		if colors[id] != "#aabbcc" {
			t.Errorf("%s = %v, want #aabbcc", id, colors[id])
		}
	}
}

// TestSetSegmentColorByGroups_EmptyColor is a no-op check.
func TestSetSegmentColorByGroups_EmptyColor(t *testing.T) {
	layer := map[string]interface{}{}
	SetSegmentColorByGroups(layer, [][]string{{"a1"}}, "")
	if _, ok := layer["segmentColors"]; ok {
		t.Error("empty color should be no-op, segmentColors should not be set")
	}
}

func TestNormalizeColorInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "named palette", input: " Blue ", want: "blue"},
		{name: "colored", input: "Colored", want: "colored"},
		{name: "hex with hash", input: "#ABCDEF", want: "#abcdef"},
		{name: "hex without hash", input: "ABCDEF", want: "#abcdef"},
		{name: "invalid short hex", input: "#fff", wantErr: true},
		{name: "invalid text", input: "black", wantErr: true},
		{name: "invalid hex chars", input: "#gg0000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeColorInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeColorInput(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NormalizeColorInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetSegmentColorByNestedGroupValues_Colored(t *testing.T) {
	// Two outer groups with "colored": outer 0 gets blue tones, outer 1 gets
	// yellow tones (with 2 groups, palettes are spaced at indices 0 and 6).
	// Inner groups take successive tones from their family.
	layer := map[string]interface{}{}
	groups := [][][]string{
		{{"a1", "a2"}, {"a3"}},
		{{"b1"}, {"b2"}},
	}

	SetSegmentColorByNestedGroupValues(layer, groups)

	got, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %T", layer["segmentColors"])
	}

	blue := colorPalettes["blue"]
	yellow := colorPalettes["yellow"]
	want := map[string]string{
		"a1": blue[0],
		"a2": blue[0],
		"a3": blue[1],
		"b1": yellow[0],
		"b2": yellow[1],
	}
	for id, hex := range want {
		if got[id] != hex {
			t.Errorf("%s = %v, want %v", id, got[id], hex)
		}
	}
}

func TestSetSegmentColorByNestedGroupValues_ToneWrapsWithinFamily(t *testing.T) {
	// A family carries five tones, so a sixth inner group reuses the first.
	layer := map[string]interface{}{}
	groups := [][][]string{{{"a1"}, {"a2"}, {"a3"}, {"a4"}, {"a5"}, {"a6"}}}

	SetSegmentColorByNestedGroupValues(layer, groups)

	got := layer["segmentColors"].(map[string]interface{})
	if got["a6"] != got["a1"] {
		t.Fatalf("a6 = %v, want the wrapped first tone %v", got["a6"], got["a1"])
	}
}

func TestSetSegmentColorByNestedGroupValues_NilGroups(t *testing.T) {
	layer := map[string]interface{}{}

	SetSegmentColorByNestedGroupValues(layer, nil)
	got, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %T", layer["segmentColors"])
	}
	if len(got) != 0 {
		t.Fatalf("segmentColors = %v, want empty", got)
	}
}

func TestSetSegmentColorByGradient_ViridisEndpoints(t *testing.T) {
	layer := map[string]interface{}{}

	SetSegmentColorByGradient(layer, map[string]float64{"low": 1, "mid": 3, "high": 5}, nil, "colored")

	got, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %T", layer["segmentColors"])
	}
	if got["low"] != viridisAnchors[0] {
		t.Errorf("low = %v, want the first viridis anchor %v", got["low"], viridisAnchors[0])
	}
	last := viridisAnchors[len(viridisAnchors)-1]
	if got["high"] != last {
		t.Errorf("high = %v, want the last viridis anchor %v", got["high"], last)
	}
	if got["mid"] == got["low"] || got["mid"] == got["high"] {
		t.Errorf("mid = %v, want a color between the ends", got["mid"])
	}
}

func TestSetSegmentColorByGradient_NamedFamilyRampsThroughItsTones(t *testing.T) {
	layer := map[string]interface{}{}

	SetSegmentColorByGradient(layer, map[string]float64{"low": 0, "mid": 1, "high": 2}, nil, "blue")

	got := layer["segmentColors"].(map[string]interface{})
	blue := colorPalettes["blue"]
	want := map[string]string{"low": blue[0], "mid": blue[2], "high": blue[4]}
	for id, hex := range want {
		if got[id] != hex {
			t.Errorf("%s = %v, want %v", id, got[id], hex)
		}
	}
}

func TestSetSegmentColorByGradient_OneValueSitsMidRamp(t *testing.T) {
	layer := map[string]interface{}{}

	SetSegmentColorByGradient(layer, map[string]float64{"a": 7, "b": 7}, nil, "blue")

	got := layer["segmentColors"].(map[string]interface{})
	blue := colorPalettes["blue"]
	if got["a"] != blue[2] || got["b"] != blue[2] {
		t.Fatalf("a=%v b=%v, want both at the mid tone %v", got["a"], got["b"], blue[2])
	}
}

func TestSetSegmentColorByGradient_UnsetTakesNeutralGray(t *testing.T) {
	layer := map[string]interface{}{}

	SetSegmentColorByGradient(layer, map[string]float64{"a": 1}, []string{"b"}, "colored")

	got := layer["segmentColors"].(map[string]interface{})
	if got["b"] != unsetValueColor {
		t.Fatalf("b = %v, want %v", got["b"], unsetValueColor)
	}
	if got["a"] == unsetValueColor {
		t.Fatalf("a = %v, want a ramp color rather than the unset gray", got["a"])
	}
}

func TestSetSegmentColorByGradient_NonFiniteValuesTakeNeutralGray(t *testing.T) {
	layer := map[string]interface{}{}

	SetSegmentColorByGradient(layer, map[string]float64{
		"low":  1,
		"high": 9,
		"nan":  math.NaN(),
		"inf":  math.Inf(1),
	}, nil, "colored")

	got := layer["segmentColors"].(map[string]interface{})
	if got["nan"] != unsetValueColor || got["inf"] != unsetValueColor {
		t.Fatalf("nan=%v inf=%v, want both %v", got["nan"], got["inf"], unsetValueColor)
	}
	if got["low"] != viridisAnchors[0] || got["high"] != viridisAnchors[len(viridisAnchors)-1] {
		t.Fatalf("finite range was distorted: low=%v high=%v", got["low"], got["high"])
	}
}

func TestSetSegmentColorByGradient_HexFlattensRampAndEmptyIsNoop(t *testing.T) {
	layer := map[string]interface{}{}

	SetSegmentColorByGradient(layer, map[string]float64{"a": 1, "b": 9}, nil, "#ff0000")

	got := layer["segmentColors"].(map[string]interface{})
	if got["a"] != "#ff0000" || got["b"] != "#ff0000" {
		t.Fatalf("a=%v b=%v, want both #ff0000", got["a"], got["b"])
	}

	empty := map[string]interface{}{}
	SetSegmentColorByGradient(empty, map[string]float64{"a": 1}, []string{"b"}, "")
	if _, ok := empty["segmentColors"]; ok {
		t.Fatal("an empty color should not create segmentColors")
	}
}
