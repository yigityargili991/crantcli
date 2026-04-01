package nglstate

import (
	"fmt"
)

// FindSegmentationLayer finds the first segmentation layer in the state, or
// one matching the given name. Returns the layer map and its index. Of course, this all is tuned to our dataset
func FindSegmentationLayer(state map[string]interface{}, layerName string) (map[string]interface{}, int, error) {
	layersRaw, ok := state["layers"]
	if !ok {
		return nil, -1, fmt.Errorf("state has no 'layers' key")
	}

	layers, ok := layersRaw.([]interface{})
	if !ok {
		return nil, -1, fmt.Errorf("'layers' is not an array")
	}

	for i, l := range layers {
		layer, ok := l.(map[string]interface{})
		if !ok {
			continue
		}

		layerType, _ := layer["type"].(string)
		if layerType != "segmentation" {
			continue
		}

		if layerName != "" {
			name, _ := layer["name"].(string)
			if name != layerName {
				continue
			}
		}

		return layer, i, nil
	}

	if layerName != "" {
		return nil, -1, fmt.Errorf("no segmentation layer named %q found", layerName)
	}
	return nil, -1, fmt.Errorf("no segmentation layer found")
}

// AddSegments adds root IDs to a segmentation layer, deduplicating.
// If replace is true, existing segments are replaced rather than appended.
func AddSegments(layer map[string]interface{}, rootIDs []string, replace bool) {
	if replace {
		layer["segments"] = toInterfaceSlice(rootIDs)
		return
	}

	existing := make(map[string]bool)
	var segments []string

	if segsRaw, ok := layer["segments"]; ok {
		if segs, ok := segsRaw.([]interface{}); ok {
			for _, s := range segs {
				str := fmt.Sprintf("%v", s)
				if !existing[str] {
					existing[str] = true
					segments = append(segments, str)
				}
			}
		}
	}

	for _, id := range rootIDs {
		if !existing[id] {
			existing[id] = true
			segments = append(segments, id)
		}
	}

	layer["segments"] = toInterfaceSlice(segments)
}

// SetSegmentColor sets the segment colors in a segmentation layer.
// color must already be normalized via NormalizeColorInput (lowercase, trimmed,
// hex prefixed with '#'). Otherwise the UI won't set it to the seed.
func SetSegmentColor(layer map[string]interface{}, rootIDs []string, color string) {
	if color == "" {
		return
	}

	// Resolve named color or "colored" before modifying segmentColors
	resolved := ResolveColor(layer, color)

	colorsRaw, ok := layer["segmentColors"]
	var colors map[string]interface{}
	if ok {
		colors, _ = colorsRaw.(map[string]interface{})
	}
	if colors == nil {
		colors = make(map[string]interface{})
	}

	for _, id := range rootIDs {
		colors[id] = resolved
	}

	layer["segmentColors"] = colors
}

func toInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
