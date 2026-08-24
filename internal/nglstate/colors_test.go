package nglstate

import (
	"fmt"
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

func TestSetSegmentColorBySubtype_ColoredMultiGroup(t *testing.T) {
	// Two groups with "colored": group 0 gets blue tones, group 1 gets yellow tones.
	// (With 2 groups, palettes are spaced at indices 0 and 6 for max contrast.)
	// Subtypes within each group get different tones from that palette.
	layer := map[string]interface{}{}
	groups := [][]string{
		{"a1", "a2", "a3", "a4"},
		{"b1", "b2", "b3"},
	}
	subtypeMap := map[string]string{
		"a1": "subtypeA",
		"a2": "subtypeB",
		"a3": "subtypeA",
		"a4": "", // no subtype
		"b1": "subtypeC",
		"b2": "subtypeD",
		"b3": "subtypeC",
	}

	// First set base group colors so neurons without subtypes have a color
	SetSegmentColorByGroups(layer, groups, "colored")
	// Then overlay subtype colors
	SetSegmentColorBySubtype(layer, groups, subtypeMap, "colored")

	got, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %T", layer["segmentColors"])
	}

	blue := colorPalettes["blue"]
	yellow := colorPalettes["yellow"]

	// Group 0 subtypes: subtypeA -> blue[0], subtypeB -> blue[1]
	if got["a1"] != blue[0] {
		t.Errorf("a1 (subtypeA) = %v, want %v", got["a1"], blue[0])
	}
	if got["a2"] != blue[1] {
		t.Errorf("a2 (subtypeB) = %v, want %v", got["a2"], blue[1])
	}
	if got["a3"] != blue[0] {
		t.Errorf("a3 (subtypeA) = %v, want %v", got["a3"], blue[0])
	}
	// a4 has no subtype: keeps its base group color from SetSegmentColorByGroups
	if got["a4"] != blue[3] {
		t.Errorf("a4 (no subtype) = %v, want base group color %v", got["a4"], blue[3])
	}

	// Group 1 subtypes: subtypeC -> yellow[0], subtypeD -> yellow[1]
	if got["b1"] != yellow[0] {
		t.Errorf("b1 (subtypeC) = %v, want %v", got["b1"], yellow[0])
	}
	if got["b2"] != yellow[1] {
		t.Errorf("b2 (subtypeD) = %v, want %v", got["b2"], yellow[1])
	}
	if got["b3"] != yellow[0] {
		t.Errorf("b3 (subtypeC) = %v, want %v", got["b3"], yellow[0])
	}
}

func TestSetSegmentColorBySubtype_NamedPalette(t *testing.T) {
	// Named palette: all groups share the same palette, subtypes get tones.
	layer := map[string]interface{}{}
	groups := [][]string{
		{"a1", "a2"},
		{"b1", "b2"},
	}
	subtypeMap := map[string]string{
		"a1": "stX",
		"a2": "stY",
		"b1": "stX", // same subtype name as group 0, but group 1 resolves independently
		"b2": "stZ",
	}

	SetSegmentColorBySubtype(layer, groups, subtypeMap, "green")

	got := layer["segmentColors"].(map[string]interface{})
	green := colorPalettes["green"]

	// Group 0: stX -> green[0], stY -> green[1]
	if got["a1"] != green[0] {
		t.Errorf("a1 = %v, want %v", got["a1"], green[0])
	}
	if got["a2"] != green[1] {
		t.Errorf("a2 = %v, want %v", got["a2"], green[1])
	}
	// Group 1: stX -> green[0], stZ -> green[1] (independent subtype order per group)
	if got["b1"] != green[0] {
		t.Errorf("b1 = %v, want %v", got["b1"], green[0])
	}
	if got["b2"] != green[1] {
		t.Errorf("b2 = %v, want %v", got["b2"], green[1])
	}
}

func TestSetSegmentColorBySubtype_HexNoop(t *testing.T) {
	// Hex color: cannot subdivide, should be a no-op for subtype coloring.
	layer := map[string]interface{}{
		"segmentColors": map[string]interface{}{"a1": "#ff0000"},
	}
	groups := [][]string{{"a1"}}
	subtypeMap := map[string]string{"a1": "stX"}

	SetSegmentColorBySubtype(layer, groups, subtypeMap, "#ff0000")

	got := layer["segmentColors"].(map[string]interface{})
	if got["a1"] != "#ff0000" {
		t.Errorf("hex subtype coloring should be no-op, got %v", got["a1"])
	}
}

func TestSetSegmentColorBySubtype_EmptyColor(t *testing.T) {
	layer := map[string]interface{}{}
	SetSegmentColorBySubtype(layer, [][]string{{"a1"}}, map[string]string{"a1": "st"}, "")
	if _, ok := layer["segmentColors"]; ok {
		t.Error("empty color should be no-op")
	}
}

func TestSetSegmentColorBySubtype_AllEmptySubtypes(t *testing.T) {
	// All neurons have empty subtypes: nothing should be overwritten.
	layer := map[string]interface{}{
		"segmentColors": map[string]interface{}{"a1": "#111111", "a2": "#222222"},
	}
	groups := [][]string{{"a1", "a2"}}
	subtypeMap := map[string]string{"a1": "", "a2": ""}

	SetSegmentColorBySubtype(layer, groups, subtypeMap, "blue")

	got := layer["segmentColors"].(map[string]interface{})
	if got["a1"] != "#111111" || got["a2"] != "#222222" {
		t.Errorf("empty subtypes should preserve existing colors, got %v", got)
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

// TestSetSegmentColorBySubtype_EmptyGroupShiftsPalette mirrors the groups test for subtype coloring.
func TestSetSegmentColorBySubtype_EmptyGroupShiftsPalette(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{
		{"a1"},
		{},
		{"b1"},
	}
	subtypeMap := map[string]string{"a1": "stA", "b1": "stB"}

	SetSegmentColorByGroups(layer, groups, "colored")
	SetSegmentColorBySubtype(layer, groups, subtypeMap, "colored")

	colors := layer["segmentColors"].(map[string]interface{})
	blue := colorPalettes["blue"]
	brown := colorPalettes["brown"] // 3 groups stride 4: indices 0,4,8 → blue, orange, brown; group 1 (empty) consumes orange

	if colors["a1"] != blue[0] {
		t.Errorf("a1 (group 0, stA) = %v, want blue[0]=%s", colors["a1"], blue[0])
	}
	if colors["b1"] != brown[0] {
		t.Errorf("b1 (group 2, stB) = %v, want brown[0]=%s (index 2*4=8 after empty group consumed index 4)", colors["b1"], brown[0])
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

// TestSetSegmentColorBySubtype_IDsNotInAnyGroup verifies that IDs present in subtypeMap
// but absent from all groups are silently ignored (no color assigned, no panic).
func TestSetSegmentColorBySubtype_IDsNotInAnyGroup(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{{"in_group"}}
	subtypeMap := map[string]string{
		"in_group":       "stA",
		"orphan_id":      "stB", // in map but not in any group
		"another_orphan": "stC",
	}
	SetSegmentColorBySubtype(layer, groups, subtypeMap, "blue")

	colors := layer["segmentColors"].(map[string]interface{})
	if _, ok := colors["orphan_id"]; ok {
		t.Errorf("orphan_id should not have a color assigned (not in any group)")
	}
	if _, ok := colors["another_orphan"]; ok {
		t.Errorf("another_orphan should not have a color assigned (not in any group)")
	}
	// in_group should have a color
	if colors["in_group"] != colorPalettes["blue"][0] {
		t.Errorf("in_group = %v, want blue[0]=%s", colors["in_group"], colorPalettes["blue"][0])
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

// TestSetSegmentColorBySubtype_SingleGroupColored verifies the single-group + "colored"
// path: group 0 always uses the blue palette for subtypes.
func TestSetSegmentColorBySubtype_SingleGroupColored(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][]string{{"n1", "n2", "n3"}}
	subtypeMap := map[string]string{
		"n1": "alpha",
		"n2": "beta",
		"n3": "alpha", // same subtype as n1
	}

	SetSegmentColorBySubtype(layer, groups, subtypeMap, "colored")

	colors := layer["segmentColors"].(map[string]interface{})
	blue := colorPalettes["blue"]

	if colors["n1"] != blue[0] {
		t.Errorf("n1 (alpha) = %v, want blue[0]=%s", colors["n1"], blue[0])
	}
	if colors["n2"] != blue[1] {
		t.Errorf("n2 (beta) = %v, want blue[1]=%s", colors["n2"], blue[1])
	}
	if colors["n3"] != blue[0] {
		t.Errorf("n3 (alpha, same as n1) = %v, want blue[0]=%s", colors["n3"], blue[0])
	}
}

// TestSetSegmentColorBySubtype_NilGroups verifies no panic on nil groups slice and that
// no IDs are unexpectedly colored (the function still writes segmentColors but it is empty).
func TestSetSegmentColorBySubtype_NilGroups(t *testing.T) {
	layer := map[string]interface{}{}
	// Must not panic
	SetSegmentColorBySubtype(layer, nil, map[string]string{"a": "st"}, "blue")
	// Function always writes segmentColors even with nil groups (consistent with SetSegmentColorByGroups).
	// What matters: no IDs from subtypeMap should have been colored.
	if colors, ok := layer["segmentColors"].(map[string]interface{}); ok {
		if len(colors) != 0 {
			t.Errorf("nil groups: no IDs should be colored, got %v", colors)
		}
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

	SetSegmentColorByNestedGroupValues(layer, groups, "colored")

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

	SetSegmentColorByNestedGroupValues(layer, groups, "colored")

	got := layer["segmentColors"].(map[string]interface{})
	if got["a6"] != got["a1"] {
		t.Fatalf("a6 = %v, want the wrapped first tone %v", got["a6"], got["a1"])
	}
}

func TestSetSegmentColorByNestedGroupValues_NamedPaletteFlattens(t *testing.T) {
	// One named family cannot carry two levels, so every inner group across the
	// whole set takes the next tone, exactly as a single-field --color-by would.
	layer := map[string]interface{}{}
	groups := [][][]string{
		{{"a1"}, {"a2"}},
		{{"b1"}},
	}

	SetSegmentColorByNestedGroupValues(layer, groups, "green")

	got := layer["segmentColors"].(map[string]interface{})
	green := colorPalettes["green"]
	want := map[string]string{"a1": green[0], "a2": green[1], "b1": green[2]}
	for id, hex := range want {
		if got[id] != hex {
			t.Errorf("%s = %v, want %v", id, got[id], hex)
		}
	}
}

func TestSetSegmentColorByNestedGroupValues_HexSharesOneColor(t *testing.T) {
	layer := map[string]interface{}{}
	groups := [][][]string{{{"a1"}, {"a2"}}, {{"b1"}}}

	SetSegmentColorByNestedGroupValues(layer, groups, "#ff0000")

	got := layer["segmentColors"].(map[string]interface{})
	for _, id := range []string{"a1", "a2", "b1"} {
		if got[id] != "#ff0000" {
			t.Errorf("%s = %v, want #ff0000", id, got[id])
		}
	}
}

func TestSetSegmentColorByNestedGroupValues_EmptyColorAndNilGroups(t *testing.T) {
	layer := map[string]interface{}{}
	SetSegmentColorByNestedGroupValues(layer, [][][]string{{{"a1"}}}, "")
	if _, ok := layer["segmentColors"]; ok {
		t.Fatal("an empty color should not create segmentColors")
	}

	SetSegmentColorByNestedGroupValues(layer, nil, "colored")
	got, ok := layer["segmentColors"].(map[string]interface{})
	if !ok {
		t.Fatalf("segmentColors missing or wrong type: %T", layer["segmentColors"])
	}
	if len(got) != 0 {
		t.Fatalf("segmentColors = %v, want empty", got)
	}
}
