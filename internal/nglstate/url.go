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

	// URL-decode the fragment
	decoded, err := url.QueryUnescape(fragment)
	if err != nil {
		// Try using the raw fragment
		decoded = fragment
	}

	var state map[string]interface{}
	if err := json.Unmarshal([]byte(decoded), &state); err != nil {
		return nil, fmt.Errorf("decoding state JSON from URL fragment: %w", err)
	}

	return state, nil
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

	return viewer + "/#!" + string(data), nil
}
