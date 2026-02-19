package nglstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	appSessionDir       = ".crantinject"
	legacyAppSessionDir = ".crant_type_look"
)

// testCachePath allows tests to override the cache directory.
// If set, it's used instead of $HOME/.crantinject
var testCachePath string

func lastStateFilePathForDir(cacheDir string) string {
	if testCachePath != "" {
		return filepath.Join(testCachePath, "last_state_url")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, cacheDir, "last_state_url")
}

func lastStateFilePath() string {
	return lastStateFilePathForDir(appSessionDir)
}

func legacyLastStateFilePath() string {
	return lastStateFilePathForDir(legacyAppSessionDir)
}

func readStateURLAtPath(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readLastStateURL() string {
	paths := []string{lastStateFilePath()}
	if legacy := legacyLastStateFilePath(); legacy != "" && legacy != paths[0] {
		paths = append(paths, legacy)
	}
	for _, path := range paths {
		if url := readStateURLAtPath(path); url != "" {
			return url
		}
	}
	return ""
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
