package nglstate

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
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
	"orange": {
		"#e65100",
		"#f57c00",
		"#fb8c00",
		"#ffb74d",
		"#ffe0b2",
	},
	"purple": {
		"#6a1b9a",
		"#8e24aa",
		"#ab47bc",
		"#ce93d8",
		"#e1bee7",
	},
	"yellow": {
		"#f9a825",
		"#fbc02d",
		"#fdd835",
		"#fff176",
		"#fff9c4",
	},
	"pink": {
		"#c2185b",
		"#d81b60",
		"#ec407a",
		"#f48fb1",
		"#f8bbd0",
	},
	"brown": {
		"#4e342e",
		"#6d4c41",
		"#8d6e63",
		"#bcaaa4",
		"#d7ccc8",
	},
	"indigo": {
		"#283593",
		"#3949ab",
		"#5c6bc0",
		"#9fa8da",
		"#c5cae9",
	},
	"teal": {
		"#00695c",
		"#00897b",
		"#26a69a",
		"#80cbc4",
		"#b2dfdb",
	},
	"lime": {
		"#9e9d24",
		"#afb42b",
		"#c0ca33",
		"#dce775",
		"#f0f4c3",
	},
}

// ResolveColor resolves a pre-normalized color input to a concrete hex code.
// colorInput must already be normalized via NormalizeColorInput (lowercase,
// trimmed, hex prefixed with '#').
//   - "colored" → random hex color
//   - Named color (see colorPalettes) → next unused tone from that palette
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
var paletteNames = []string{
	"blue", "red", "green", "turquoise",
	"orange", "purple", "yellow", "pink",
	"brown", "indigo", "teal", "lime",
}

var categoricalColors = []string{
	"#e6194b", "#3cb44b", "#4363d8", "#f58231",
	"#911eb4", "#46f0f0", "#f032e6", "#bcf60c",
	"#fabebe", "#008080", "#e6beff", "#9a6324",
	"#fffac8", "#800000", "#aaffc3", "#808000",
	"#ffd8b1", "#000075", "#a9a9a9", "#ff6f00",
	"#00b8d4", "#c51162", "#64dd17", "#6200ea",
	"#00c853", "#d50000", "#2962ff", "#ffd600",
	"#00bfa5", "#aa00ff", "#ffab00", "#0091ea",
	"#aeea00", "#dd2c00", "#304ffe", "#00b0ff",
	"#ff4081", "#76ff03", "#ffea00", "#1b5e20",
	"#8d6e63", "#ad1457", "#26a69a", "#5e35b1",
	"#ef6c00", "#558b2f", "#d81b60", "#3949ab",
}

// groupPaletteDistinct returns the palette index for a group, spacing groups
// evenly across the available palettes for maximum visual distinctiveness.
// With N groups, palette indices are spaced by floor(P/N) positions.
func groupPaletteDistinct(groupIdx, numGroups int) int {
	if numGroups <= 1 {
		return 0
	}
	stride := len(paletteNames) / numGroups
	if stride < 1 {
		stride = 1
	}
	return (groupIdx * stride) % len(paletteNames)
}

// SetSegmentColorByGroups assigns colors to multiple groups of segments.
// Each group represents a cell type. colorInput must already be normalized via
// NormalizeColorInput (lowercase, trimmed, hex prefixed with '#').
// With "colored", each group gets a distinct palette (spaced around the color
// wheel for maximum visual separation) and neurons within cycle through its tones.
// With a named color, all groups share that palette with tones continuing across
// groups.
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
		// Each group gets a different palette, spaced for maximum distinctness.
		// Neurons within a group cycle through the palette's tones.
		numGroups := len(groups)
		for i, group := range groups {
			paletteIdx := groupPaletteDistinct(i, numGroups)
			palette := colorPalettes[paletteNames[paletteIdx]]
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

// SetSegmentColorByGroupValues assigns one color per group. This is used for
// --color-by fields where each group is a semantic value, such as a column.
func SetSegmentColorByGroupValues(layer map[string]interface{}, groups [][]string, colorInput string) {
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

	for i, group := range groups {
		groupColor := colorInput
		switch {
		case colorInput == "colored":
			groupColor = categoricalGroupColor(i)
		case colorPalettes[colorInput] != nil:
			palette := colorPalettes[colorInput]
			groupColor = palette[i%len(palette)]
		}
		for _, id := range group {
			colors[id] = groupColor
		}
	}

	layer["segmentColors"] = colors
}

func categoricalGroupColor(groupIdx int) string {
	if groupIdx < len(categoricalColors) {
		return categoricalColors[groupIdx]
	}

	const goldenAngle = 137.508
	hue := math.Mod(float64(groupIdx-len(categoricalColors))*goldenAngle, 360)
	lightness := 0.48
	if groupIdx%2 == 1 {
		lightness = 0.62
	}
	return hslToHex(hue, 0.82, lightness)
}

func hslToHex(h, s, l float64) string {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}

	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	return fmt.Sprintf("#%02x%02x%02x", colorByte(r+m), colorByte(g+m), colorByte(b+m))
}

func colorByte(v float64) int {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return int(math.Round(v * 255))
}

// SetSegmentColorBySubtype assigns sub-colors to neurons within each group
// based on their cell_subtype. Each group gets its own base palette (determined
// by the group index, spaced for maximum visual separation). Within a group,
// each distinct subtype gets its own tone. Neurons with empty subtypes keep
// their existing group color.
func SetSegmentColorBySubtype(layer map[string]interface{}, groups [][]string, subtypeMap map[string]string, colorInput string) {
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

	for i, group := range groups {
		var palette []string
		switch {
		case colorInput == "colored":
			numGroups := len(groups)
			paletteIdx := groupPaletteDistinct(i, numGroups)
			palette = colorPalettes[paletteNames[paletteIdx]]
		case colorPalettes[colorInput] != nil:
			palette = colorPalettes[colorInput]
		default:
			// Single hex color -- cannot sub-color, skip
			continue
		}

		// Collect distinct subtypes, sorted alphabetically for deterministic
		// color assignment regardless of query result ordering.
		subtypeSeen := map[string]bool{}
		for _, id := range group {
			st := subtypeMap[id]
			if st != "" {
				subtypeSeen[st] = true
			}
		}
		subtypeOrder := make([]string, 0, len(subtypeSeen))
		for st := range subtypeSeen {
			subtypeOrder = append(subtypeOrder, st)
		}
		sort.Strings(subtypeOrder)

		subtypeToTone := make(map[string]int, len(subtypeOrder))
		for idx, st := range subtypeOrder {
			subtypeToTone[st] = idx % len(palette)
		}

		for _, id := range group {
			st := subtypeMap[id]
			if st == "" {
				continue
			}
			colors[id] = palette[subtypeToTone[st]]
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

	hex := strings.TrimPrefix(trimmed, "#")
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
