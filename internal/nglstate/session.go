package nglstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// testCachePath allows tests to override the cache directory.
// If set, it's used instead of $HOME/.crant_type_look
var testCachePath string

func lastStateFilePath() string {
	if testCachePath != "" {
		return filepath.Join(testCachePath, "last_state_url")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".crant_type_look", "last_state_url")
}

func readLastStateURL() string {
	path := lastStateFilePath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeLastStateURL(rawURL string) error {
	path := lastStateFilePath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating state cache directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(strings.TrimSpace(rawURL)+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing state cache: %w", err)
	}
	return nil
}
