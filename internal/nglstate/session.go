package nglstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	appSessionDir        = ".crantcli"
	legacyAppSessionDir  = ".crantinject"
	legacyAppSessionDir2 = ".crant_type_look"
)

// testCachePath allows tests to override the cache directory.
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
	for _, dir := range []string{appSessionDir, legacyAppSessionDir, legacyAppSessionDir2} {
		if url := readStateURLAtPath(lastStateFilePathForDir(dir)); url != "" {
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

func defaultStateFilePath() string {
	if testCachePath != "" {
		return filepath.Join(testCachePath, "default_state.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, appSessionDir, "default_state.json")
}

// ReadDefaultState returns the user-configured default state JSON, if any.
func ReadDefaultState() ([]byte, error) {
	path := defaultStateFilePath()
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// WriteDefaultState saves JSON as the new default state.
// If jsonData is nil, the custom default is removed.
func WriteDefaultState(jsonData []byte) error {
	path := defaultStateFilePath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}

	if jsonData == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing custom default state: %w", err)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0o600); err != nil {
		return fmt.Errorf("writing default state: %w", err)
	}
	return nil
}
