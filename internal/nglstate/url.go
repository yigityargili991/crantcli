package nglstate

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// IsNeuroglancerURL checks if a string looks like a Neuroglancer URL.
func IsNeuroglancerURL(s string) bool {
	return strings.Contains(s, "neuroglancer") ||
		strings.Contains(s, "spelunker") ||
		strings.Contains(s, "cave-explorer") ||
		(strings.HasPrefix(s, "http") && strings.Contains(s, "#!"))
}

// DecodeURL extracts the JSON state from a Neuroglancer URL fragment.
func DecodeURL(rawURL string) (map[string]interface{}, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	fragment := u.Fragment
	if fragment == "" {
		return nil, fmt.Errorf("URL has no fragment (expected #!{...} or #!url)")
	}

	// Strip leading '!' if present
	fragment = strings.TrimPrefix(fragment, "!")
	if isRemoteStateFragment(fragment) {
		return nil, fmt.Errorf("remote Neuroglancer state URLs (#!url...) are not supported; provide an embedded-state URL or JSON file")
	}

	// URL-decode the fragment.
	// Use PathUnescape so '+' is preserved (QueryUnescape converts '+' to space).
	decoded, err := url.PathUnescape(fragment)
	if err != nil {
		// Try using the raw fragment
		decoded = fragment
	}
	if isRemoteStateFragment(decoded) {
		return nil, fmt.Errorf("remote Neuroglancer state URLs (#!url...) are not supported; provide an embedded-state URL or JSON file")
	}

	var state map[string]interface{}
	if err := json.Unmarshal([]byte(decoded), &state); err != nil {
		return nil, fmt.Errorf("decoding state JSON from URL fragment: %w", err)
	}
	repairLegacyMiddleauthSource(state)

	return state, nil
}

func isRemoteStateFragment(fragment string) bool {
	fragment = strings.TrimSpace(fragment)
	if len(fragment) < len("url") || !strings.EqualFold(fragment[:len("url")], "url") {
		return false
	}
	if len(fragment) == len("url") {
		return true
	}
	switch fragment[len("url")] {
	case ':', '=', '/', '?':
		return true
	default:
		return false
	}
}

// EncodeURL creates a Neuroglancer URL with the state embedded in the fragment.
func EncodeURL(state map[string]interface{}, viewer string) (string, error) {
	if viewer == "" {
		viewer = "https://spelunker.cave-explorer.org"
	}

	data, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshaling state: %w", err)
	}

	return viewer + "/#!" + url.PathEscape(string(data)), nil
}

// repairLegacyMiddleauthSource fixes old state URLs that were decoded with '+'
// converted to spaces in graphene source URLs.
func repairLegacyMiddleauthSource(state map[string]interface{}) {
	layersRaw, ok := state["layers"]
	if !ok {
		return
	}

	layers, ok := layersRaw.([]interface{})
	if !ok {
		return
	}

	for _, l := range layers {
		layer, ok := l.(map[string]interface{})
		if !ok {
			continue
		}
		src, ok := layer["source"].(string)
		if !ok {
			continue
		}
		if strings.Contains(src, "graphene://middleauth https://") {
			layer["source"] = strings.ReplaceAll(src, "graphene://middleauth https://", "graphene://middleauth+https://")
		}
	}
}
