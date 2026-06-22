package nglstate

import (
	"fmt"
	"strings"
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

// ReplaceSegments replaces segment IDs that appear in mappings and deduplicates
// the final segment list while preserving the first-seen order.
func ReplaceSegments(layer map[string]interface{}, mappings map[string]string) int {
	if len(mappings) == 0 {
		return 0
	}

	var segments []string
	seen := make(map[string]bool)
	replaced := 0
	colors := segmentColors(layer)

	if segsRaw, ok := layer["segments"]; ok {
		if segs, ok := segsRaw.([]interface{}); ok {
			for _, s := range segs {
				str := fmt.Sprintf("%v", s)
				next := str
				if mapped, ok := mappings[str]; ok {
					next = mapped
					replaced++
					migrateSegmentColor(colors, str, next)
				}
				if !seen[next] {
					seen[next] = true
					segments = append(segments, next)
				}
			}
		}
	}

	removeMappedSegmentColors(colors, mappings)
	layer["segments"] = toInterfaceSlice(segments)
	return replaced
}

func segmentColors(layer map[string]interface{}) map[string]interface{} {
	colorsRaw, ok := layer["segmentColors"]
	if !ok {
		return nil
	}
	colors, _ := colorsRaw.(map[string]interface{})
	return colors
}

func migrateSegmentColor(colors map[string]interface{}, oldID, newID string) {
	if colors == nil || oldID == newID {
		return
	}
	color, ok := colors[oldID]
	if !ok {
		return
	}
	if _, exists := colors[newID]; !exists {
		colors[newID] = color
	}
}

func removeMappedSegmentColors(colors map[string]interface{}, mappings map[string]string) {
	if colors == nil {
		return
	}
	for oldID := range mappings {
		delete(colors, oldID)
	}
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

// EnsureSegmentPropertiesSource attaches a precomputed segment-properties source
// to a segmentation layer so per-segment labels render in the Seg. panel. It
// normalizes the layer's existing source (string, object, or array) into an
// array and preserves the primary segmentation source(s) (the graphene
// segmentation must remain first). Any previously attached label source — one
// whose URL is listed in priorURLs, or a crantcli-managed gist source — is
// removed first, so repeated --labels runs replace rather than accumulate stale
// sources (which would otherwise become dead once their host is cleaned up).
// Adding the same URL twice is a no-op.
func EnsureSegmentPropertiesSource(layer map[string]interface{}, propertiesURL string, priorURLs []string) error {
	if propertiesURL == "" {
		return fmt.Errorf("properties URL is empty")
	}

	existing, ok := layer["source"]
	if !ok {
		return fmt.Errorf("layer has no 'source' to attach properties to")
	}

	var sources []interface{}
	switch s := existing.(type) {
	case string:
		sources = []interface{}{s}
	case map[string]interface{}:
		sources = []interface{}{s}
	case []interface{}:
		sources = s
	default:
		return fmt.Errorf("unsupported source type %T", existing)
	}

	prior := make(map[string]bool, len(priorURLs))
	for _, u := range priorURLs {
		prior[u] = true
	}

	kept := make([]interface{}, 0, len(sources)+1)
	for _, s := range sources {
		u := sourceURL(s)
		if u != propertiesURL && (prior[u] || isManagedLabelSource(u)) {
			continue
		}
		kept = append(kept, s)
	}
	if !sourceListContainsURL(kept, propertiesURL) {
		kept = append(kept, map[string]interface{}{"url": propertiesURL})
	}
	layer["source"] = kept
	return nil
}

// sourceURL extracts the URL from a source entry (string or object form).
func sourceURL(s interface{}) string {
	switch v := s.(type) {
	case string:
		return v
	case map[string]interface{}:
		u, _ := v["url"].(string)
		return u
	default:
		return ""
	}
}

// sourceListContainsURL reports whether any source entry points at the given URL.
func sourceListContainsURL(sources []interface{}, url string) bool {
	for _, s := range sources {
		if sourceURL(s) == url {
			return true
		}
	}
	return false
}

// isManagedLabelSource reports whether a source URL is a crantcli-managed
// segment-properties label source (a gist-hosted properties map). These are
// replaced, not accumulated, on each --labels run.
func isManagedLabelSource(url string) bool {
	return strings.Contains(url, "gist.githubusercontent.com") &&
		strings.HasSuffix(url, "|neuroglancer-precomputed:")
}

func toInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
