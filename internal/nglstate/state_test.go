package nglstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadState_PrefersExistingFileEvenIfNameLooksLikeURL(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "neuroglancer_state.json")
	if err := os.WriteFile(statePath, []byte(`{"layers":[]}`), 0o600); err != nil {
		t.Fatalf("failed to write test state file: %v", err)
	}

	result, err := LoadState(statePath, false)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if result.Source != SourceFile {
		t.Fatalf("expected SourceFile, got %v", result.Source)
	}
	if result.State == nil {
		t.Fatal("expected non-nil state")
	}
	if _, ok := result.State["layers"]; !ok {
		t.Fatal("expected state to contain layers key")
	}
}
