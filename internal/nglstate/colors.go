package nglstate

import (
	"fmt"
	"math/rand"
	"strings"
)

// colorPalettes maps a named color family to an ordered list of hex tones
// from darker/more saturated to lighter/less saturated.
var colorPalettes = map[string][]string{
	"blue": {
		"#1565c0",
		"#1e88e5",
		"#42a5f5",
		"#90caf9",
		"#bbdefb",
	},
	"red": {
		"#c62828",
		"#e53935",
		"#ef5350",
		"#ef9a9a",
		"#ffcdd2",
	},
	"green": {
		"#2e7d32",
		"#43a047",
		"#66bb6a",
		"#a5d6a7",
		"#c8e6c9",
	},
	"turquoise": {
		"#00838f",
		"#00acc1",
		"#26c6da",
		"#80deea",
		"#b2ebf2",
	},
}

// ResolveColor resolves a pre-normalized color input to a concrete hex code.
// colorInput must already be normalized via NormalizeColorInput (lowercase,
// trimmed, hex prefixed with '#').
//   - "colored" → random hex color
//   - Named color (blue, red, green, turquoise) → next unused tone from that palette
//   - Raw hex → returned as-is
//   - Empty → empty
func ResolveColor(layer map[string]interface{}, colorInput string) string {
	if colorInput == "" {
		return ""
	}

	if colorInput == "colored" {
		return randomColor()
	}

	palette, ok := colorPalettes[colorInput]
	if ok {
		usedCount := countPaletteTones(layer, palette)
		idx := usedCount % len(palette)
		return palette[idx]
	}

	// Already-normalized hex — return as-is
	return colorInput
}

// countPaletteTones counts how many distinct tones from the given palette
// are already present as values in the layer's segmentColors map.
func countPaletteTones(layer map[string]interface{}, palette []string) int {
	colorsRaw, ok := layer["segmentColors"]
	if !ok {
		return 0
	}
	colors, ok := colorsRaw.(map[string]interface{})
	if !ok {
		return 0
	}

	paletteSet := make(map[string]bool, len(palette))
	for _, hex := range palette {
		paletteSet[strings.ToLower(hex)] = true
	}

	seen := make(map[string]bool)
	for _, v := range colors {
		hex, ok := v.(string)
		if !ok {
			continue
		}
		lower := strings.ToLower(hex)
		if paletteSet[lower] && !seen[lower] {
			seen[lower] = true
		}
	}
	return len(seen)
}

// paletteNames is the ordered list of palettes for automatic per-type assignment.
var paletteNames = []string{"blue", "red", "green", "turquoise"}

// SetSegmentColorByGroups assigns colors to multiple groups of segments.
// Each group represents a cell type. colorInput must already be normalized via
// NormalizeColorInput (lowercase, trimmed, hex prefixed with '#').
// With "colored", each group gets a distinct palette and neurons within cycle
// through its tones. With a named color, all groups share that palette with
// tones continuing across groups.
func SetSegmentColorByGroups(layer map[string]interface{}, groups [][]string, colorInput string) {
	if colorInput == "" {
		return
	}

	colorsRaw, ok := layer["segmentColors"]
	var colors map[string]interface{}
	if ok {
		colors, _ = colorsRaw.(map[string]interface{})
	}
	if colors == nil {
		colors = make(map[string]interface{})
	}

	switch {
	case colorInput == "colored":
		// Each group gets a different palette, neurons cycle through tones
		for i, group := range groups {
			palette := colorPalettes[paletteNames[i%len(paletteNames)]]
			for j, id := range group {
				colors[id] = palette[j%len(palette)]
			}
		}
	case colorPalettes[colorInput] != nil:
		// Named color: all groups share the palette, tones continue across groups
		palette := colorPalettes[colorInput]
		toneIdx := 0
		for _, group := range groups {
			for _, id := range group {
				colors[id] = palette[toneIdx%len(palette)]
				toneIdx++
			}
		}
	default:
		// Already-normalized hex color: all neurons get the same color
		for _, group := range groups {
			for _, id := range group {
				colors[id] = colorInput
			}
		}
	}

	layer["segmentColors"] = colors
}

// randomColor generates a random hex color string.
func randomColor() string {
	return fmt.Sprintf("#%06x", rand.Intn(0xFFFFFF)+1)
}

// NormalizeColorInput validates and normalizes a color input.
func NormalizeColorInput(colorInput string) (string, error) {
	trimmed := strings.TrimSpace(colorInput)
	if trimmed == "" {
		return "", nil
	}

	normalized := strings.ToLower(trimmed)
	if normalized == "colored" {
		return normalized, nil
	}
	if _, ok := colorPalettes[normalized]; ok {
		return normalized, nil
	}

	hex := trimmed
	if strings.HasPrefix(hex, "#") {
		hex = hex[1:]
	}
	if len(hex) != 6 || !isHexString(hex) {
		return "", fmt.Errorf("invalid color %q: use a named palette, 'colored', or a 6-digit hex value like #ff0000", colorInput)
	}

	return "#" + strings.ToLower(hex), nil
}

func isHexString(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
