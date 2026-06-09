package nglstate

import (
	"os"
	"path/filepath"
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

func TestLoadState_UsesLastStateBeforeTemplate(t *testing.T) {
	setupStateFlowTest(t)
	lastURL, err := EncodeURL(map[string]interface{}{"layers": []interface{}{}, "source": "last"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLastStateURL(lastURL); err != nil {
		t.Fatal(err)
	}

	result, err := LoadState("", false)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if result.Source != SourceLastState {
		t.Fatalf("Source = %q, want last-state", result.Source)
	}
	if result.OriginalURL != lastURL {
		t.Fatalf("OriginalURL = %q, want %q", result.OriginalURL, lastURL)
	}
	if result.State["source"] != "last" {
		t.Fatalf("state = %#v, want last-state data", result.State)
	}
}

func TestLoadStateErrorsOnMalformedClipboardNeuroglancerURL(t *testing.T) {
	setupStateFlowTest(t)
	clipboardRead = func() (string, error) {
		return "https://spelunker.cave-explorer.org/#!not-json", nil
	}
	lastURL, err := EncodeURL(map[string]interface{}{"layers": []interface{}{}, "source": "last"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLastStateURL(lastURL); err != nil {
		t.Fatal(err)
	}

	_, err = LoadState("", false)
	if err == nil {
		t.Fatal("expected malformed clipboard URL error")
	}
	if !strings.Contains(err.Error(), "decoding clipboard Neuroglancer URL") {
		t.Fatalf("error = %q, want clipboard decode context", err.Error())
	}
}

func TestLoadStateErrorsOnMalformedLastStateNeuroglancerURL(t *testing.T) {
	setupStateFlowTest(t)
	if err := writeLastStateURL("https://spelunker.cave-explorer.org/#!not-json"); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState("", false)
	if err == nil {
		t.Fatal("expected malformed last-state URL error")
	}
	if !strings.Contains(err.Error(), "decoding last-session Neuroglancer URL") {
		t.Fatalf("error = %q, want last-session decode context", err.Error())
	}
}

func TestLoadStateFallsBackForNonURLClipboardAndLastState(t *testing.T) {
	setupStateFlowTest(t)
	clipboardRead = func() (string, error) { return "720575940610453042", nil }
	if err := writeLastStateURL("plain root ids"); err != nil {
		t.Fatal(err)
	}

	result, err := LoadState("", false)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if result.Source != SourceTemplate {
		t.Fatalf("Source = %q, want template fallback", result.Source)
	}
	if result.State == nil {
		t.Fatal("expected built-in template state")
	}
}

func TestLoadStateGenerateSkipsImplicitSources(t *testing.T) {
	setupStateFlowTest(t)
	clipboardURL, err := EncodeURL(map[string]interface{}{"layers": []interface{}{}, "source": "clipboard"}, "")
	if err != nil {
		t.Fatal(err)
	}
	clipboardRead = func() (string, error) { return clipboardURL, nil }
	lastURL, err := EncodeURL(map[string]interface{}{"layers": []interface{}{}, "source": "last"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLastStateURL(lastURL); err != nil {
		t.Fatal(err)
	}
	if err := WriteDefaultState([]byte(`{"layers":[{"type":"segmentation"}],"source":"default"}`)); err != nil {
		t.Fatal(err)
	}

	result, err := LoadState("", true)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if result.Source != SourceTemplate {
		t.Fatalf("Source = %q, want template", result.Source)
	}
	if result.State["source"] != "default" {
		t.Fatalf("state = %#v, want configured default state", result.State)
	}
}

func TestLoadStateExplicitStateWinsOverGenerate(t *testing.T) {
	setupStateFlowTest(t)
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(statePath, []byte(`{"layers":[],"source":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDefaultState([]byte(`{"layers":[{"type":"segmentation"}],"source":"default"}`)); err != nil {
		t.Fatal(err)
	}

	result, err := LoadState(statePath, true)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if result.Source != SourceFile {
		t.Fatalf("Source = %q, want file", result.Source)
	}
	if result.State["source"] != "file" {
		t.Fatalf("state = %#v, want explicit file state", result.State)
	}
}

func TestWriteStatePersistsURL(t *testing.T) {
	setupStateFlowTest(t)
	var copied string
	clipboardWrite = func(value string) error {
		copied = value
		return nil
	}
	result := &LoadResult{
		State:  map[string]interface{}{"layers": []interface{}{}, "source": "template"},
		Source: SourceTemplate,
	}

	if err := WriteState(result, ""); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}
	if result.OutputURL == "" {
		t.Fatal("OutputURL was not set")
	}
	if copied != result.OutputURL {
		t.Fatalf("clipboard = %q, want %q", copied, result.OutputURL)
	}
	if got := readLastStateURL(); got != result.OutputURL {
		t.Fatalf("last state = %q, want %q", got, result.OutputURL)
	}
}

func TestWriteStateDoesNotPersistFileOutput(t *testing.T) {
	setupStateFlowTest(t)
	outputPath := filepath.Join(t.TempDir(), "state.json")
	result := &LoadResult{
		State:  map[string]interface{}{"layers": []interface{}{}},
		Source: SourceTemplate,
	}

	if err := WriteState(result, outputPath); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}
	if got := readLastStateURL(); got != "" {
		t.Fatalf("last state = %q, want empty", got)
	}
}

func setupStateFlowTest(t *testing.T) {
	t.Helper()
	oldCache := testCachePath
	oldRead := clipboardRead
	oldWrite := clipboardWrite
	testCachePath = t.TempDir()
	clipboardRead = func() (string, error) { return "", nil }
	clipboardWrite = func(string) error { return nil }
	t.Cleanup(func() {
		testCachePath = oldCache
		clipboardRead = oldRead
		clipboardWrite = oldWrite
	})
}
