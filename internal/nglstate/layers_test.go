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
			name:     "set color without hash",
			layer:    map[string]interface{}{},
			rootIDs:  []string{"123"},
			color:    "ff0000",
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
