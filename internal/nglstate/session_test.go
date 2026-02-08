package nglstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLastStateFilePath(t *testing.T) {
	t.Run("returns empty string when HOME cannot be determined", func(t *testing.T) {
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)

		// Unset HOME to simulate error
		os.Unsetenv("HOME")

		path := lastStateFilePath()
		if path != "" {
			t.Errorf("expected empty string when HOME not set, got %q", path)
		}
	})

	t.Run("returns correct path when HOME is set", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		path := lastStateFilePath()
		expected := filepath.Join(tempHome, ".crant_type_look", "last_state_url")
		if path != expected {
			t.Errorf("expected %q, got %q", expected, path)
		}
	})
}

func TestReadLastStateURL(t *testing.T) {
	t.Run("returns empty string when file does not exist", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		url := readLastStateURL()
		if url != "" {
			t.Errorf("expected empty string when file does not exist, got %q", url)
		}
	})

	t.Run("returns empty string when HOME cannot be determined", func(t *testing.T) {
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)

		// Unset HOME to simulate error
		os.Unsetenv("HOME")

		url := readLastStateURL()
		if url != "" {
			t.Errorf("expected empty string when HOME not set, got %q", url)
		}
	})

	t.Run("reads and trims whitespace from file", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		// Create the cache directory and file
		cacheDir := filepath.Join(tempHome, ".crant_type_look")
		if err := os.MkdirAll(cacheDir, 0o700); err != nil {
			t.Fatal(err)
		}

		testURL := "https://example.com/neuroglancer/#!{}"
		testData := "  " + testURL + "  \n\t"
		filePath := filepath.Join(cacheDir, "last_state_url")
		if err := os.WriteFile(filePath, []byte(testData), 0o600); err != nil {
			t.Fatal(err)
		}

		url := readLastStateURL()
		if url != testURL {
			t.Errorf("expected %q, got %q", testURL, url)
		}
	})

	t.Run("handles newlines and tabs", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		// Create the cache directory and file
		cacheDir := filepath.Join(tempHome, ".crant_type_look")
		if err := os.MkdirAll(cacheDir, 0o700); err != nil {
			t.Fatal(err)
		}

		testURL := "https://example.com/test"
		testData := "\n\t" + testURL + "\n\n"
		filePath := filepath.Join(cacheDir, "last_state_url")
		if err := os.WriteFile(filePath, []byte(testData), 0o600); err != nil {
			t.Fatal(err)
		}

		url := readLastStateURL()
		if url != testURL {
			t.Errorf("expected %q, got %q", testURL, url)
		}
	})
}

func TestWriteLastStateURL(t *testing.T) {
	t.Run("returns error when HOME cannot be determined", func(t *testing.T) {
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)

		// Unset HOME to simulate error
		os.Unsetenv("HOME")

		err := writeLastStateURL("https://example.com/test")
		if err == nil {
			t.Error("expected error when HOME not set")
		}
		if err != nil && err.Error() != "could not determine home directory" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("creates directory with correct permissions", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		testURL := "https://example.com/neuroglancer/#!{}"
		err := writeLastStateURL(testURL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check that directory was created with correct permissions
		cacheDir := filepath.Join(tempHome, ".crant_type_look")
		info, err := os.Stat(cacheDir)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}
		// Check permissions (0o700)
		perm := info.Mode().Perm()
		if perm != 0o700 {
			t.Errorf("expected directory permissions 0o700, got 0o%o", perm)
		}
	})

	t.Run("writes file with correct permissions", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		testURL := "https://example.com/neuroglancer/#!{}"
		err := writeLastStateURL(testURL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check that file was created with correct permissions
		filePath := filepath.Join(tempHome, ".crant_type_look", "last_state_url")
		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("file not created: %v", err)
		}
		if info.IsDir() {
			t.Error("expected file, not directory")
		}
		// Check permissions (0o600)
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("expected file permissions 0o600, got 0o%o", perm)
		}
	})

	t.Run("trims whitespace and adds newline", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		testURL := "https://example.com/neuroglancer/#!{}"
		inputURL := "  " + testURL + "  \n\t"
		err := writeLastStateURL(inputURL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Read back and verify
		filePath := filepath.Join(tempHome, ".crant_type_look", "last_state_url")
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		expected := testURL + "\n"
		if string(data) != expected {
			t.Errorf("expected %q, got %q", expected, string(data))
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		// Create temp directory for HOME
		tempHome := t.TempDir()
		
		// Save original HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		
		os.Setenv("HOME", tempHome)

		// Write first URL
		firstURL := "https://example.com/first"
		err := writeLastStateURL(firstURL)
		if err != nil {
			t.Fatalf("unexpected error on first write: %v", err)
		}

		// Write second URL
		secondURL := "https://example.com/second"
		err = writeLastStateURL(secondURL)
		if err != nil {
			t.Fatalf("unexpected error on second write: %v", err)
		}

		// Read back and verify it's the second URL
		url := readLastStateURL()
		if url != secondURL {
			t.Errorf("expected %q, got %q", secondURL, url)
		}
	})
}

func TestWriteReadRoundTrip(t *testing.T) {
	// Create temp directory for HOME
	tempHome := t.TempDir()
	
	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	
	os.Setenv("HOME", tempHome)

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "simple_url",
			url:  "https://example.com/neuroglancer/#!{}",
		},
		{
			name: "url_with_layers",
			url:  "https://spelunker.cave-explorer.org/#!{\"layers\":[]}",
		},
		{
			name: "url_with_position",
			url:  "https://neuroglancer-demo.appspot.com/#!{\"position\":[1,2,3]}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeLastStateURL(tt.url)
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}

			url := readLastStateURL()
			if url != tt.url {
				t.Errorf("round-trip failed: wrote %q, read %q", tt.url, url)
			}
		})
	}
}
