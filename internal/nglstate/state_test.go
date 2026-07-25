package nglstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestLoadStateFromURL(t *testing.T) {
	const stateURL = `https://example.org/#!{"layers":[],"position":[1,2,3]}`

	result, err := LoadState(stateURL, false)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if result.Source != SourceURL {
		t.Fatalf("source = %q, want %q", result.Source, SourceURL)
	}
	if result.OriginalURL != stateURL {
		t.Fatalf("original URL = %q, want %q", result.OriginalURL, stateURL)
	}
	if _, ok := result.State["position"]; !ok {
		t.Fatal("decoded state is missing position")
	}
}

func TestLoadStateFromStdin(t *testing.T) {
	replaceStdin(t, `{"layers":[],"from":"stdin"}`)

	result, err := LoadState("", true)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if result.Source != SourceStdin {
		t.Fatalf("source = %q, want %q", result.Source, SourceStdin)
	}
	if got := result.State["from"]; got != "stdin" {
		t.Fatalf("state from = %#v, want stdin", got)
	}
}

func TestLoadStateUsesCustomDefault(t *testing.T) {
	previousCachePath := testCachePath
	testCachePath = t.TempDir()
	t.Cleanup(func() {
		testCachePath = previousCachePath
	})
	replaceStdin(t, "")

	if err := WriteDefaultState([]byte(`{"layers":[],"custom":true}`)); err != nil {
		t.Fatalf("WriteDefaultState: %v", err)
	}
	result, err := LoadState("", true)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if result.Source != SourceTemplate {
		t.Fatalf("source = %q, want %q", result.Source, SourceTemplate)
	}
	if got := result.State["custom"]; got != true {
		t.Fatalf("custom = %#v, want true", got)
	}
}

func TestLoadStateExplicitInputErrors(t *testing.T) {
	t.Run("invalid file JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatalf("write state: %v", err)
		}

		_, err := LoadState(path, false)
		if err == nil || !strings.Contains(err.Error(), "parsing state file") {
			t.Fatalf("LoadState() error = %v, want parsing state file error", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.json")

		_, err := LoadState(path, false)
		if err == nil || !strings.Contains(err.Error(), "reading state file") {
			t.Fatalf("LoadState() error = %v, want reading state file error", err)
		}
	})

	t.Run("invalid URL state", func(t *testing.T) {
		_, err := LoadState("https://example.org/#!not-json", false)
		if err == nil || !strings.Contains(err.Error(), "decoding URL") {
			t.Fatalf("LoadState() error = %v, want decoding URL error", err)
		}
	})
}

func TestWriteStateToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	result := &LoadResult{
		State: map[string]interface{}{
			"layers": []interface{}{},
			"zoom":   float64(4),
		},
		Source: SourceStdin,
	}

	if err := WriteState(result, path); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output state: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatal("output state does not end with a newline")
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got["zoom"] != float64(4) {
		t.Fatalf("zoom = %#v, want 4", got["zoom"])
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat output state: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("output state permissions = 0o%o, want 0600", perm)
		}
	}
}

func replaceStdin(t *testing.T, content string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write stdin fixture: %v", err)
	}
	input, err := os.Open(path)
	if err != nil {
		t.Fatalf("open stdin fixture: %v", err)
	}

	previousStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = previousStdin
		_ = input.Close()
	})
}
