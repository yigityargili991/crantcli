package cmd

import "testing"

func TestFormatLayerSource(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := formatLayerSource(nil); got != "" {
			t.Fatalf("expected empty source, got %q", got)
		}
	})

	t.Run("string", func(t *testing.T) {
		const src = "graphene://middleauth+https://example.org/table/x"
		if got := formatLayerSource(src); got != src {
			t.Fatalf("expected %q, got %q", src, got)
		}
	})

	t.Run("object", func(t *testing.T) {
		src := map[string]interface{}{
			"url": "graphene://middleauth+https://example.org/table/x",
		}
		want := `{"url":"graphene://middleauth+https://example.org/table/x"}`
		if got := formatLayerSource(src); got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})
}

func TestCountSegments(t *testing.T) {
	tests := []struct {
		name  string
		layer map[string]interface{}
		want  int
	}{
		{name: "missing", layer: map[string]interface{}{}, want: 0},
		{name: "invalid type", layer: map[string]interface{}{"segments": "123"}, want: 0},
		{name: "empty", layer: map[string]interface{}{"segments": []interface{}{}}, want: 0},
		{name: "populated", layer: map[string]interface{}{"segments": []interface{}{"123", "456"}}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countSegments(tt.layer); got != tt.want {
				t.Fatalf("countSegments() = %d, want %d", got, tt.want)
			}
		})
	}
}
