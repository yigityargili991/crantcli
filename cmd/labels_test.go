package cmd

import "testing"

func TestResolveLabelsHook(t *testing.T) {
	t.Setenv("CRANT_LABELS_HOOK", "env-hook")

	if got := resolveLabelsHook("flag-hook"); got != "flag-hook" {
		t.Fatalf("flag value should win, got %q", got)
	}
	if got := resolveLabelsHook(""); got != "env-hook" {
		t.Fatalf("environment value should be the fallback, got %q", got)
	}
}
