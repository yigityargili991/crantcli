package nglstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// errNoFragment marks a URL that carries no state fragment at all, as opposed
// to one whose fragment cannot be decoded. Callers can treat the former as an
// accidental paste (the bare viewer link) and the latter as real corruption.
var errNoFragment = errors.New("URL has no fragment (expected #!{...} or #!url)")

// splitFragment separates a URL into the part before the first '#' and the
// still-encoded fragment after it, mirroring how a browser fills location.hash.
// url.Parse is deliberately avoided: it percent-decodes the fragment, and it
// rejects the whole URL outright when a state contains a literal '%' that is not
// a valid escape (a layer named "100% confidence", say).
func splitFragment(rawURL string) (string, string, bool) {
	base, fragment, found := strings.Cut(rawURL, "#")
	return base, fragment, found
}

// IsNeuroglancerURL checks if a string looks like a Neuroglancer URL.
func IsNeuroglancerURL(s string) bool {
	base, fragment, _ := splitFragment(s)
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	location := strings.ToLower(u.Host + u.Path)
	return strings.Contains(location, "neuroglancer") ||
		strings.Contains(location, "spelunker") ||
		strings.Contains(location, "cave-explorer") ||
		strings.HasPrefix(fragment, "!")
}

// DecodeURL extracts the JSON state from a Neuroglancer URL fragment.
func DecodeURL(rawURL string) (map[string]interface{}, error) {
	_, fragment, found := splitFragment(rawURL)
	if !found || fragment == "" {
		return nil, errNoFragment
	}
	fragment = strings.TrimPrefix(fragment, "!")

	// Mirror the viewer, which percent-decodes the fragment before parsing it.
	// PathUnescape rather than QueryUnescape, so a '+' in a graphene source
	// survives instead of becoming a space.
	if decoded, err := url.PathUnescape(fragment); err == nil {
		if state, parseErr := parseStateJSON(decoded); parseErr == nil {
			return state, nil
		}
	}

	// PathUnescape rejects a fragment holding a bare '%', which is exactly what
	// older crantcli versions and hand-edited URLs emit. Read those as raw JSON
	// rather than refusing a URL the user can see is well formed.
	state, err := parseStateJSON(fragment)
	if err != nil {
		return nil, fmt.Errorf("decoding state JSON from URL fragment: %w", err)
	}
	return state, nil
}

func parseStateJSON(fragment string) (map[string]interface{}, error) {
	var state map[string]interface{}
	if err := json.Unmarshal([]byte(fragment), &state); err != nil {
		return nil, err
	}
	repairLegacyMiddleauthSource(state)
	return state, nil
}

// EncodeURL creates a Neuroglancer URL with the state embedded in the fragment.
func EncodeURL(state map[string]interface{}, viewer string) (string, error) {
	if viewer == "" {
		viewer = "https://spelunker.cave-explorer.org"
	}

	base, err := url.Parse(viewer)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return "", fmt.Errorf("invalid Neuroglancer viewer URL %q", viewer)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshaling state: %w", err)
	}

	// Neuroglancer percent-decodes the fragment, so a literal '%' in the state
	// (a layer named "100% confidence", a shader, an annotation) is read as the
	// start of an escape sequence and the viewer reports "URI malformed".
	// Escaping '%' and nothing else keeps the fragment decodable while leaving
	// every state without a '%' byte-identical to the URL the viewer itself
	// produces -- spaces, '#', quotes, and non-ASCII all stay literal, because
	// those already round-trip through the viewer today.
	return strings.TrimSuffix(viewer, "/") + "/#!" + strings.ReplaceAll(string(data), "%", "%25"), nil
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
