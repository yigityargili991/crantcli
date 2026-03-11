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

// ResolveColor resolves a color input to a concrete hex code.
//   - "colored" → random hex color
//   - Named color (blue, red, green, turquoise) → next unused tone from that palette
//   - Raw hex → returned as-is with '#' prefix ensured
//   - Empty → empty
func ResolveColor(layer map[string]interface{}, colorInput string) string {
	if colorInput == "" {
		return ""
	}

	normalized := strings.ToLower(strings.TrimSpace(colorInput))

	if normalized == "colored" {
		return randomColor()
	}

	palette, ok := colorPalettes[normalized]
	if ok {
		usedCount := countPaletteTones(layer, palette)
		idx := usedCount % len(palette)
		return palette[idx]
	}

	// Raw hex — ensure # prefix
	if !strings.HasPrefix(colorInput, "#") {
		return "#" + colorInput
	}
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

// randomColor generates a random hex color string.
func randomColor() string {
	return fmt.Sprintf("#%06x", rand.Intn(0xFFFFFF+1))
}
