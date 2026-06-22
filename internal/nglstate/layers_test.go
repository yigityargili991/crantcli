package nglstate

import (
	"reflect"
	"testing"
)

func TestAddSegments(t *testing.T) {
	tests := []struct {
		name     string
		layer    map[string]interface{}
		rootIDs  []string
		replace  bool
		expected []interface{}
	}{
		{
			name:     "add to empty",
			layer:    map[string]interface{}{"segments": []interface{}{}},
			rootIDs:  []string{"123", "456"},
			expected: []interface{}{"123", "456"},
		},
		{
			name:     "append to existing",
			layer:    map[string]interface{}{"segments": []interface{}{"123"}},
			rootIDs:  []string{"456"},
			expected: []interface{}{"123", "456"},
		},
		{
			name:     "deduplicate",
			layer:    map[string]interface{}{"segments": []interface{}{"123"}},
			rootIDs:  []string{"123", "456"},
			expected: []interface{}{"123", "456"},
		},
		{
			name:     "replace mode",
			layer:    map[string]interface{}{"segments": []interface{}{"old1", "old2"}},
			rootIDs:  []string{"new1", "new2"},
			replace:  true,
			expected: []interface{}{"new1", "new2"},
		},
		{
			name:     "no existing segments",
			layer:    map[string]interface{}{},
			rootIDs:  []string{"123"},
			expected: []interface{}{"123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AddSegments(tt.layer, tt.rootIDs, tt.replace)
			got := tt.layer["segments"]
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEnsureSegmentPropertiesSource(t *testing.T) {
	const url = "precomputed://https://host/raw/sha/|neuroglancer-precomputed:"
	grapheneObj := map[string]interface{}{
		"url":   "graphene://middleauth+https://x/segmentation/table/y",
		"state": map[string]interface{}{"merge": map[string]interface{}{}},
	}

	tests := []struct {
		name     string
		layer    map[string]interface{}
		expected []interface{}
		wantErr  bool
	}{
		{
			name:     "string source becomes array",
			layer:    map[string]interface{}{"source": "graphene://x"},
			expected: []interface{}{"graphene://x", map[string]interface{}{"url": url}},
		},
		{
			name:     "object source preserved at index 0",
			layer:    map[string]interface{}{"source": grapheneObj},
			expected: []interface{}{grapheneObj, map[string]interface{}{"url": url}},
		},
		{
			name:     "appends to existing array",
			layer:    map[string]interface{}{"source": []interface{}{"graphene://x"}},
			expected: []interface{}{"graphene://x", map[string]interface{}{"url": url}},
		},
		{
			name:     "idempotent when url already present as object",
			layer:    map[string]interface{}{"source": []interface{}{"graphene://x", map[string]interface{}{"url": url}}},
			expected: []interface{}{"graphene://x", map[string]interface{}{"url": url}},
		},
		{
			name:     "idempotent when url already present as string",
			layer:    map[string]interface{}{"source": []interface{}{url}},
			expected: []interface{}{url},
		},
		{
			name:    "missing source errors",
			layer:   map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "unsupported source type errors",
			layer:   map[string]interface{}{"source": 42},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsureSegmentPropertiesSource(tt.layer, url, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := tt.layer["source"]
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEnsureSegmentPropertiesSource_EmptyURL(t *testing.T) {
	layer := map[string]interface{}{"source": "graphene://x"}
	if err := EnsureSegmentPropertiesSource(layer, "", nil); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestEnsureSegmentPropertiesSource_ReplacesPriorManagedGist(t *testing.T) {
	// Gist sources are recognized by heuristic even without priorURLs.
	oldURL := "https://gist.githubusercontent.com/u/OLD/raw/sha/|neuroglancer-precomputed:"
	newURL := "https://gist.githubusercontent.com/u/NEW/raw/sha/|neuroglancer-precomputed:"
	layer := map[string]interface{}{
		"source": []interface{}{
			"graphene://x",
			map[string]interface{}{"url": oldURL},
		},
	}

	if err := EnsureSegmentPropertiesSource(layer, newURL, nil); err != nil {
		t.Fatal(err)
	}

	want := []interface{}{"graphene://x", map[string]interface{}{"url": newURL}}
	if got := layer["source"]; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (old gist source should be replaced)", got, want)
	}
}

func TestEnsureSegmentPropertiesSource_ReplacesPriorURL(t *testing.T) {
	// Non-gist (hook) sources are recognized via priorURLs.
	oldURL := "https://my-host.example/labels/v1/|neuroglancer-precomputed:"
	newURL := "https://my-host.example/labels/v2/|neuroglancer-precomputed:"
	layer := map[string]interface{}{
		"source": []interface{}{
			"graphene://x",
			map[string]interface{}{"url": oldURL},
		},
	}

	if err := EnsureSegmentPropertiesSource(layer, newURL, []string{oldURL}); err != nil {
		t.Fatal(err)
	}

	want := []interface{}{"graphene://x", map[string]interface{}{"url": newURL}}
	if got := layer["source"]; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (old hook source should be replaced via priorURLs)", got, want)
	}
}

func TestReplaceSegments(t *testing.T) {
	tests := []struct {
		name         string
		layer        map[string]interface{}
		mappings     map[string]string
		wantSegments []interface{}
		wantReplaced int
	}{
		{
			name:         "replaces stale IDs and preserves others",
			layer:        map[string]interface{}{"segments": []interface{}{"old1", "keep", "old2"}},
			mappings:     map[string]string{"old1": "new1", "old2": "new2"},
			wantSegments: []interface{}{"new1", "keep", "new2"},
			wantReplaced: 2,
		},
		{
			name:         "deduplicates current IDs",
			layer:        map[string]interface{}{"segments": []interface{}{"old1", "new1", "old2", "keep"}},
			mappings:     map[string]string{"old1": "new1", "old2": "new1"},
			wantSegments: []interface{}{"new1", "keep"},
			wantReplaced: 2,
		},
		{
			name:         "handles numeric segment values",
			layer:        map[string]interface{}{"segments": []interface{}{float64(123), "keep"}},
			mappings:     map[string]string{"123": "456"},
			wantSegments: []interface{}{"456", "keep"},
			wantReplaced: 1,
		},
		{
			name:         "empty mappings leave layer unchanged",
			layer:        map[string]interface{}{"segments": []interface{}{"old1"}},
			mappings:     map[string]string{},
			wantSegments: []interface{}{"old1"},
			wantReplaced: 0,
		},
		{
			name:         "missing segments becomes empty list",
			layer:        map[string]interface{}{},
			mappings:     map[string]string{"old1": "new1"},
			wantSegments: []interface{}{},
			wantReplaced: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReplaced := ReplaceSegments(tt.layer, tt.mappings)
			if gotReplaced != tt.wantReplaced {
				t.Fatalf("replaced = %d, want %d", gotReplaced, tt.wantReplaced)
			}
			if got := tt.layer["segments"]; !reflect.DeepEqual(got, tt.wantSegments) {
				t.Fatalf("segments = %#v, want %#v", got, tt.wantSegments)
			}
		})
	}
}

func TestReplaceSegments_MigratesSegmentColors(t *testing.T) {
	layer := map[string]interface{}{
		"segments": []interface{}{"old1", "new1", "old2", "keep"},
		"segmentColors": map[string]interface{}{
			"old1": "#ff0000",
			"new1": "#00ff00",
			"old2": "#0000ff",
			"keep": "#ffffff",
		},
	}

	replaced := ReplaceSegments(layer, map[string]string{
		"old1": "new1",
		"old2": "new2",
	})
	if replaced != 2 {
		t.Fatalf("replaced = %d, want 2", replaced)
	}

	wantSegments := []interface{}{"new1", "new2", "keep"}
	if got := layer["segments"]; !reflect.DeepEqual(got, wantSegments) {
		t.Fatalf("segments = %#v, want %#v", got, wantSegments)
	}

	wantColors := map[string]interface{}{
		"new1": "#00ff00",
		"new2": "#0000ff",
		"keep": "#ffffff",
	}
	if got := layer["segmentColors"]; !reflect.DeepEqual(got, wantColors) {
		t.Fatalf("segmentColors = %#v, want %#v", got, wantColors)
	}
}

func TestSetSegmentColor(t *testing.T) {
	tests := []struct {
		name         string
		layer        map[string]interface{}
		rootIDs      []string
		color        string
		expected     map[string]interface{}
		checkMissing bool
	}{
		{
			name:     "set color with hash",
			layer:    map[string]interface{}{},
			rootIDs:  []string{"123", "456"},
			color:    "#ff0000",
			expected: map[string]interface{}{"123": "#ff0000", "456": "#ff0000"},
		},
		{
			name:     "set pre-normalized hex color",
			layer:    map[string]interface{}{},
			rootIDs:  []string{"123"},
			color:    "#ff0000",
			expected: map[string]interface{}{"123": "#ff0000"},
		},
		{
			name:         "empty color does nothing",
			layer:        map[string]interface{}{},
			rootIDs:      []string{"123"},
			color:        "",
			checkMissing: true,
		},
		{
			name:     "merge with existing colors",
			layer:    map[string]interface{}{"segmentColors": map[string]interface{}{"existing": "#00ff00"}},
			rootIDs:  []string{"123"},
			color:    "#ff0000",
			expected: map[string]interface{}{"existing": "#00ff00", "123": "#ff0000"},
		},
		{
			name:     "named color blue resolves to first tone",
			layer:    map[string]interface{}{},
			rootIDs:  []string{"123"},
			color:    "blue",
			expected: map[string]interface{}{"123": "#1565c0"},
		},
		{
			name:     "named color blue cycles to second tone",
			layer:    map[string]interface{}{"segmentColors": map[string]interface{}{"existing": "#1565c0"}},
			rootIDs:  []string{"456"},
			color:    "blue",
			expected: map[string]interface{}{"existing": "#1565c0", "456": "#1e88e5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetSegmentColor(tt.layer, tt.rootIDs, tt.color)
			if tt.checkMissing {
				if _, ok := tt.layer["segmentColors"]; ok {
					t.Errorf("expected segmentColors to not be set")
				}
				return
			}
			got := tt.layer["segmentColors"]
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFindSegmentationLayer(t *testing.T) {
	state := map[string]interface{}{
		"layers": []interface{}{
			map[string]interface{}{"name": "image", "type": "image"},
			map[string]interface{}{"name": "seg1", "type": "segmentation"},
			map[string]interface{}{"name": "seg2", "type": "segmentation"},
		},
	}

	t.Run("find first segmentation layer", func(t *testing.T) {
		layer, idx, err := FindSegmentationLayer(state, "")
		if err != nil {
			t.Fatal(err)
		}
		if idx != 1 {
			t.Errorf("expected index 1, got %d", idx)
		}
		if layer["name"] != "seg1" {
			t.Errorf("expected seg1, got %v", layer["name"])
		}
	})

	t.Run("find by name", func(t *testing.T) {
		layer, idx, err := FindSegmentationLayer(state, "seg2")
		if err != nil {
			t.Fatal(err)
		}
		if idx != 2 {
			t.Errorf("expected index 2, got %d", idx)
		}
		if layer["name"] != "seg2" {
			t.Errorf("expected seg2, got %v", layer["name"])
		}
	})

	t.Run("name not found", func(t *testing.T) {
		_, _, err := FindSegmentationLayer(state, "nonexistent")
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("no layers key", func(t *testing.T) {
		_, _, err := FindSegmentationLayer(map[string]interface{}{}, "")
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("no segmentation layer", func(t *testing.T) {
		state := map[string]interface{}{
			"layers": []interface{}{
				map[string]interface{}{"name": "image", "type": "image"},
			},
		}
		_, _, err := FindSegmentationLayer(state, "")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestSetSegmentColorByGroups(t *testing.T) {
	t.Run("colored assigns palette per group", func(t *testing.T) {
		layer := map[string]interface{}{}
		groups := [][]string{
			{"a1", "a2"},
			{"b1", "b2"},
		}

		SetSegmentColorByGroups(layer, groups, "colored")

		got, ok := layer["segmentColors"].(map[string]interface{})
		if !ok {
			t.Fatalf("segmentColors missing or wrong type: %T", layer["segmentColors"])
		}

		want := map[string]interface{}{
			"a1": colorPalettes["blue"][0],
			"a2": colorPalettes["blue"][1],
			"b1": colorPalettes["yellow"][0], // 2 groups stride 6: index 6 → yellow
			"b2": colorPalettes["yellow"][1],
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("segmentColors = %#v, want %#v", got, want)
		}
	})

	t.Run("invalid color is rejected by NormalizeColorInput", func(t *testing.T) {
		// Invalid colors are now caught by NormalizeColorInput before reaching
		// SetSegmentColorByGroups, so we test the normalization layer instead.
		_, err := NormalizeColorInput("not-a-color")
		if err == nil {
			t.Fatalf("expected NormalizeColorInput(\"not-a-color\") to return error")
		}
	})
}
