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
