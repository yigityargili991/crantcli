package nglstate

import (
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
			name:    "no fragment",
			input:   "https://example.org/",
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   "https://example.org/#!not-json",
			wantErr: true,
		},
		{
			name:    "remote state url unsupported",
			input:   "https://example.org/#!url=https://state.example/state.json",
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
		if !strings.Contains(got, "%7B%22layers%22:%5B%5D%7D") {
			t.Errorf("expected encoded JSON fragment, got %q", got)
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
}

func TestEncodeURLSpecialCharactersRoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"layers": []interface{}{
			map[string]interface{}{
				"source": "graphene://middleauth+https://example.org/a path?q=1&x=2",
				"type":   "segmentation",
			},
		},
	}

	url, err := EncodeURL(original, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(url, "{\"layers\"") {
		t.Fatalf("URL fragment was not encoded: %q", url)
	}
	if !strings.Contains(url, "middleauth+https") {
		t.Fatalf("PathEscape should preserve plus signs in fragments, got %q", url)
	}

	decoded, err := DecodeURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("decoded = %#v, want %#v", decoded, original)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"layers": []interface{}{
			map[string]interface{}{"type": "segmentation"},
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
