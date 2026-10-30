// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package nglstate

import (
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestIsNeuroglancerURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://spelunker.cave-explorer.org/#!{}", true},
		{"https://neuroglancer-demo.appspot.com/#!{}", true},
		{"http://example.com/neuroglancer/#!{}", true},
		{"https://example.com/#!{}", true},
		{"just some text", false},
		{"/tmp/neuroglancer_state.json", false},
		{`C:\tmp\spelunker_state.json`, false},
		{"https://example.com/no-fragment", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsNeuroglancerURL(tt.input)
			if got != tt.want {
				t.Errorf("IsNeuroglancerURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]interface{}
		wantErr bool
	}{
		{
			name:  "simple json fragment",
			input: "https://example.org/#!{\"layers\":[]}",
			want:  map[string]interface{}{"layers": []interface{}{}},
		},
		{
			name:  "url encoded fragment",
			input: "https://example.org/#!%7B%22layers%22%3A%5B%5D%7D",
			want:  map[string]interface{}{"layers": []interface{}{}},
		},
		{
			// Older crantcli versions and hand-edited URLs carry raw JSON with a
			// bare '%'; percent-decoding rejects it, so it is read as-is.
			name:  "raw json fragment with a bare percent",
			input: `https://example.org/#!{"name":"100% confidence"}`,
			want:  map[string]interface{}{"name": "100% confidence"},
		},
		{
			name:  "url encoded state holding a share link",
			input: "https://example.org/#!%7B%22source%22%3A%22middleauth%2Bhttps%3A%2F%2Fx%22%7D",
			want:  map[string]interface{}{"source": "middleauth+https://x"},
		},
		{
			name:  "plus signs are preserved in fragment json",
			input: "https://example.org/#!{\"source\":\"graphene://middleauth+https://x\"}",
			want:  map[string]interface{}{"source": "graphene://middleauth+https://x"},
		},
		{
			name:  "repairs legacy middleauth source with spaces",
			input: "https://example.org/#!{\"layers\":[{\"source\":\"graphene://middleauth https://x\",\"type\":\"segmentation\"}]}",
			want: map[string]interface{}{
				"layers": []interface{}{
					map[string]interface{}{
						"source": "graphene://middleauth+https://x",
						"type":   "segmentation",
					},
				},
			},
		},
		{
			name:  "repairs legacy middleauth object source",
			input: `https://example.org/#!{"layers":[{"source":{"url":"graphene://middleauth https://x","subsources":{"default":true}},"type":"segmentation"}]}`,
			want: map[string]interface{}{
				"layers": []interface{}{
					map[string]interface{}{
						"source": map[string]interface{}{
							"url":        "graphene://middleauth+https://x",
							"subsources": map[string]interface{}{"default": true},
						},
						"type": "segmentation",
					},
				},
			},
		},
		{
			// The embedded CRANT scene lists its graphene source this way.
			name:  "repairs legacy middleauth sources in a list",
			input: `https://example.org/#!{"layers":[{"source":[{"url":"graphene://middleauth https://x"},"graphene://middleauth https://y","https://z"],"type":"segmentation"}]}`,
			want: map[string]interface{}{
				"layers": []interface{}{
					map[string]interface{}{
						"source": []interface{}{
							map[string]interface{}{"url": "graphene://middleauth+https://x"},
							"graphene://middleauth+https://y",
							"https://z",
						},
						"type": "segmentation",
					},
				},
			},
		},
		{
			name:    "no fragment",
			input:   "https://example.org/",
			wantErr: true,
		},
		{
			name:    "null state",
			input:   "https://example.org/#!null",
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   "https://example.org/#!not-json",
			wantErr: true,
		},
		{
			name:    "invalid url",
			input:   "://invalid-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DecodeURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeURLRemoteState(t *testing.T) {
	// The viewer's Share button copies a link naming where the state is
	// stored. It is not malformed JSON and must not be reported as such. The
	// location is kept exactly as the link spells it, escapes included:
	// decoding a%2Fb would name a different URL.
	for _, location := range []string{
		"middleauth+https://global.daf-apis.com/nglstate/api/v1/5195345298243584",
		"https://example.org/state.json",
		"gs://bucket/state.json",
		"middleauth+https://global.daf-ap%C4%B0s.com/nglstate/api/v1/42",
		"https://example.org/a%2Fb/state.json",
		"https://example.org/100%.json",
	} {
		t.Run(location, func(t *testing.T) {
			_, err := DecodeURL("https://spelunker.cave-explorer.org/#!" + location)
			var remote *remoteStateError
			if !errors.As(err, &remote) || remote.location != location {
				t.Fatalf("DecodeURL() error = %v, want a remote state error for %q", err, location)
			}
			if strings.Contains(err.Error(), "invalid character") {
				t.Fatalf("DecodeURL() error = %q reads as malformed JSON", err)
			}
		})
	}
}

func TestDecodeURLPercentEncodedRemoteState(t *testing.T) {
	// URL encoders and bug-report formatters percent-encode a Share link's
	// fragment. Decoded, it names the same location as the plain link.
	tests := []struct {
		fragment, location string
	}{
		{
			"middleauth%2Bhttps%3A%2F%2Fglobal.daf-apis.com%2Fnglstate%2Fapi%2Fv1%2F5195345298243584",
			"middleauth+https://global.daf-apis.com/nglstate/api/v1/5195345298243584",
		},
		{
			"middleauth+https%3A%2F%2Fglobal.daf-apis.com/nglstate/api/v1/5195345298243584",
			"middleauth+https://global.daf-apis.com/nglstate/api/v1/5195345298243584",
		},
		{"https%3A%2F%2Fexample.org%2Fstate.json", "https://example.org/state.json"},
		{"gs%3A%2F%2Fbucket%2Fstate.json", "gs://bucket/state.json"},
	}
	for _, tt := range tests {
		t.Run(tt.fragment, func(t *testing.T) {
			_, err := DecodeURL("https://spelunker.cave-explorer.org/#!" + tt.fragment)
			var remote *remoteStateError
			if !errors.As(err, &remote) || remote.location != tt.location {
				t.Fatalf("DecodeURL() error = %v, want a remote state error for %q", err, tt.location)
			}
		})
	}
}

func TestDecodeURLMalformedState(t *testing.T) {
	// A fragment that names no location, even once decoded, is read as a
	// state, and one that does not parse is reported as malformed JSON.
	tests := []struct {
		fragment, want string
	}{
		{`{"layers":[`, "decoding state JSON from URL fragment: unexpected end of JSON input"},
		{"%7B%22layers%22%3A%5B", "decoding state JSON from URL fragment: invalid character '%' looking for beginning of value"},
		{"middleauth%2Bhttps%3A%2Fglobal.daf-apis.com", "decoding state JSON from URL fragment: invalid character 'm' looking for beginning of value"},
	}
	for _, tt := range tests {
		t.Run(tt.fragment, func(t *testing.T) {
			_, err := DecodeURL("https://spelunker.cave-explorer.org/#!" + tt.fragment)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("DecodeURL() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEncodeURL(t *testing.T) {
	state := map[string]interface{}{"layers": []interface{}{}}

	t.Run("default viewer", func(t *testing.T) {
		got, err := EncodeURL(state, "")
		if err != nil {
			t.Fatal(err)
		}
		wantPrefix := "https://spelunker.cave-explorer.org/#!"
		if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
			t.Errorf("expected prefix %q, got %q", wantPrefix, got)
		}
	})

	t.Run("custom viewer", func(t *testing.T) {
		got, err := EncodeURL(state, "https://custom.viewer.org")
		if err != nil {
			t.Fatal(err)
		}
		wantPrefix := "https://custom.viewer.org/#!"
		if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
			t.Errorf("expected prefix %q, got %q", wantPrefix, got)
		}
	})

	t.Run("only percent is escaped", func(t *testing.T) {
		got, err := EncodeURL(map[string]interface{}{"value": `quoted value | 100% #tag`}, "")
		if err != nil {
			t.Fatal(err)
		}
		// A bare '%' makes the viewer's decodeURIComponent throw "URI
		// malformed". Everything else stays literal, because it already
		// round-trips through the viewer today.
		want := `#!{"value":"quoted value | 100%25 #tag"}`
		if !strings.HasSuffix(got, want) {
			t.Fatalf("URL = %q, want it to end with %q", got, want)
		}
	})

	t.Run("states without percent are byte-identical to the legacy format", func(t *testing.T) {
		got, err := EncodeURL(state, "")
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		want := "https://spelunker.cave-explorer.org/#!" + string(data)
		if got != want {
			t.Fatalf("URL = %q, want the unchanged legacy concatenation %q", got, want)
		}
	})

	t.Run("fragment is valid decodeURIComponent input", func(t *testing.T) {
		got, err := EncodeURL(map[string]interface{}{"name": `100% sure`, "note": "50%off"}, "")
		if err != nil {
			t.Fatal(err)
		}
		_, fragment, _ := splitFragment(got)
		if _, err := url.PathUnescape(strings.TrimPrefix(fragment, "!")); err != nil {
			t.Fatalf("viewer would report URI malformed: %v (fragment %q)", err, fragment)
		}
	})

	t.Run("no double slash when the viewer has a trailing slash", func(t *testing.T) {
		got, err := EncodeURL(state, "https://custom.viewer.org/")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "org//#!") {
			t.Fatalf("URL = %q, want a single slash before the fragment", got)
		}
	})

	t.Run("literal percent and hash survive a round trip", func(t *testing.T) {
		original := map[string]interface{}{"name": `100% confidence #3`, "shader": "void main() {\n}"}
		encoded, err := EncodeURL(original, "")
		if err != nil {
			t.Fatal(err)
		}
		if !IsNeuroglancerURL(encoded) {
			t.Fatalf("EncodeURL produced a URL that IsNeuroglancerURL rejects: %q", encoded)
		}
		decoded, err := DecodeURL(encoded)
		if err != nil {
			t.Fatalf("DecodeURL(%q) = %v", encoded, err)
		}
		if !reflect.DeepEqual(decoded, original) {
			t.Fatalf("round trip = %#v, want %#v", decoded, original)
		}
	})

	t.Run("invalid viewer", func(t *testing.T) {
		if _, err := EncodeURL(state, "not a URL"); err == nil {
			t.Fatal("EncodeURL accepted an invalid viewer")
		}
	})
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"layers": []interface{}{
			map[string]interface{}{"type": "segmentation", "source": "https://example.org/a%2Fb?q=a+b"},
		},
		"position": []interface{}{float64(1), float64(2), float64(3)},
	}

	url, err := EncodeURL(original, "")
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeURL(url)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("round-trip failed: got %v, want %v", decoded, original)
	}
}
