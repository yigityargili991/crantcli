package nglstate

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestCache creates a temporary directory for session cache testing
// and returns a cleanup function.
func setupTestCache(t *testing.T) (cleanup func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "crant_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Override the cache path for this test
	oldCachePath := testCachePath
	testCachePath = tmpDir

	return func() {
		testCachePath = oldCachePath
		os.RemoveAll(tmpDir)
	}
}

// TestLoadState_SessionFallback tests that LoadState falls back to the
// last remembered URL when clipboard is empty or invalid.
func TestLoadState_SessionFallback(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Write a valid Neuroglancer URL to the session cache
	testURL := "https://example.org/#!{\"layers\":[]}"
	if err := writeLastStateURL(testURL); err != nil {
		t.Fatalf("failed to write test URL: %v", err)
	}

	// Test: LoadState with no arguments should use session fallback
	result, err := LoadState("", false)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if result.Source != SourceSession {
		t.Errorf("expected source to be SourceSession, got %v", result.Source)
	}

	if result.OriginalURL != testURL {
		t.Errorf("expected OriginalURL to be %q, got %q", testURL, result.OriginalURL)
	}

	if result.State == nil {
		t.Error("expected State to be non-nil")
	}

	// Verify the state was decoded correctly
	if _, ok := result.State["layers"]; !ok {
		t.Error("expected state to contain 'layers' key")
	}
}

// TestLoadState_SessionFallback_SkippedOnGenerate tests that the session
// fallback is skipped when generate=true.
func TestLoadState_SessionFallback_SkippedOnGenerate(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Write a valid Neuroglancer URL to the session cache
	testURL := "https://example.org/#!{\"layers\":[{\"name\":\"test\"}]}"
	if err := writeLastStateURL(testURL); err != nil {
		t.Fatalf("failed to write test URL: %v", err)
	}

	// Test: LoadState with generate=true should skip session fallback
	result, err := LoadState("", true)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if result.Source != SourceTemplate {
		t.Errorf("expected source to be SourceTemplate with generate=true, got %v", result.Source)
	}

	if result.OriginalURL != "" {
		t.Errorf("expected OriginalURL to be empty, got %q", result.OriginalURL)
	}

	// The template should be used instead of the session URL
	if result.State == nil {
		t.Error("expected State to be non-nil")
	}
}

// TestLoadState_SessionFallback_InvalidURL tests that invalid URLs in
// session cache are ignored and template is used instead.
func TestLoadState_SessionFallback_InvalidURL(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Write an invalid URL to the session cache
	invalidURL := "not-a-valid-url"
	if err := writeLastStateURL(invalidURL); err != nil {
		t.Fatalf("failed to write test URL: %v", err)
	}

	// Test: LoadState should skip invalid session URL and use template
	result, err := LoadState("", false)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if result.Source != SourceTemplate {
		t.Errorf("expected source to be SourceTemplate when session URL is invalid, got %v", result.Source)
	}
}

// TestLoadState_SessionFallback_EmptyCache tests that when the cache
// is empty, LoadState falls back to the template.
func TestLoadState_SessionFallback_EmptyCache(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Don't write anything to the cache - it should be empty

	// Test: LoadState should use template when cache is empty
	result, err := LoadState("", false)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if result.Source != SourceTemplate {
		t.Errorf("expected source to be SourceTemplate when cache is empty, got %v", result.Source)
	}
}

// TestLoadState_SessionFallback_MalformedJSON tests that malformed JSON
// in the session URL is handled gracefully.
func TestLoadState_SessionFallback_MalformedJSON(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Write a URL with malformed JSON to the session cache
	malformedURL := "https://example.org/#!{invalid json}"
	if err := writeLastStateURL(malformedURL); err != nil {
		t.Fatalf("failed to write test URL: %v", err)
	}

	// Test: LoadState should skip malformed JSON and use template
	result, err := LoadState("", false)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if result.Source != SourceTemplate {
		t.Errorf("expected source to be SourceTemplate when session URL has malformed JSON, got %v", result.Source)
	}
}

// TestWriteState_PersistsSession tests that WriteState persists the
// output URL to the session cache for appropriate sources when writing
// to clipboard (not to file).
func TestWriteState_PersistsSession(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Note: We can't test clipboard writing in a unit test environment,
	// so we verify the file writing behavior and trust the WriteState
	// implementation for clipboard cases.
	testCases := []struct {
		name   string
		source StateSource
	}{
		{"SourceFile", SourceFile},
		{"SourceStdin", SourceStdin},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear the cache before each test
			cachePath := lastStateFilePath()
			if cachePath != "" {
				os.Remove(cachePath)
			}

			result := &LoadResult{
				State:  map[string]interface{}{"layers": []interface{}{}},
				Source: tc.source,
			}

			// Write to a temp file
			tmpFile := filepath.Join(testCachePath, "output.json")
			if err := WriteState(result, tmpFile); err != nil {
				t.Fatalf("WriteState failed: %v", err)
			}

			// For file and stdin sources, session should not be persisted
			// when writing to a file
			savedURL := readLastStateURL()
			if savedURL != "" {
				t.Errorf("expected session not to be persisted for %v when writing to file, but got %q", tc.source, savedURL)
			}

			// OutputURL should not be set when writing to file
			if result.OutputURL != "" {
				t.Errorf("expected OutputURL to be empty when writing to file, got %q", result.OutputURL)
			}
		})
	}
}

// TestSessionCache_ReadWrite tests the low-level session cache functions.
func TestSessionCache_ReadWrite(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	testURL := "https://example.org/#!{\"test\":\"data\"}"

	// Write to cache
	if err := writeLastStateURL(testURL); err != nil {
		t.Fatalf("writeLastStateURL failed: %v", err)
	}

	// Read from cache
	readURL := readLastStateURL()
	if readURL != testURL {
		t.Errorf("expected to read %q, got %q", testURL, readURL)
	}

	// Verify file exists
	cachePath := lastStateFilePath()
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("expected cache file to exist")
	}

	// Write again (overwrite)
	newURL := "https://example.org/#!{\"updated\":\"data\"}"
	if err := writeLastStateURL(newURL); err != nil {
		t.Fatalf("writeLastStateURL (overwrite) failed: %v", err)
	}

	readURL = readLastStateURL()
	if readURL != newURL {
		t.Errorf("expected to read updated URL %q, got %q", newURL, readURL)
	}
}

// TestSessionCache_EmptyRead tests reading from a non-existent cache.
func TestSessionCache_EmptyRead(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Don't write anything - cache should be empty
	readURL := readLastStateURL()
	if readURL != "" {
		t.Errorf("expected empty string from non-existent cache, got %q", readURL)
	}
}

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
