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
