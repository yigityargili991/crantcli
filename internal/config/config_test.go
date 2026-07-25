package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func setupTestHome(t *testing.T) (string, func()) {
	t.Helper()
	tempHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tempHome); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	cleanup := func() {
		_ = os.Setenv("HOME", originalHome)
	}
	return tempHome, cleanup
}

func writeEncodedToken(t *testing.T, path, token string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

func TestReadStoredToken(t *testing.T) {
	t.Run("reads new config path", func(t *testing.T) {
		home, cleanup := setupTestHome(t)
		defer cleanup()

		writeEncodedToken(t, filepath.Join(home, ".crantcli", "credentials"), "new-token")

		if got := ReadStoredToken(); got != "new-token" {
			t.Fatalf("expected new-token, got %q", got)
		}
	})

	t.Run("falls back to legacy config path", func(t *testing.T) {
		home, cleanup := setupTestHome(t)
		defer cleanup()

		writeEncodedToken(t, filepath.Join(home, ".crant_type_look", "credentials"), "legacy-token")

		if got := ReadStoredToken(); got != "legacy-token" {
			t.Fatalf("expected legacy-token, got %q", got)
		}
	})

	t.Run("prefers new config path over legacy", func(t *testing.T) {
		home, cleanup := setupTestHome(t)
		defer cleanup()

		writeEncodedToken(t, filepath.Join(home, ".crantcli", "credentials"), "new-token")
		writeEncodedToken(t, filepath.Join(home, ".crant_type_look", "credentials"), "legacy-token")

		if got := ReadStoredToken(); got != "new-token" {
			t.Fatalf("expected new-token, got %q", got)
		}
	})
}

func TestReadStoredToken_TightensLoosePermissions(t *testing.T) {
	home, cleanup := setupTestHome(t)
	defer cleanup()

	path := filepath.Join(home, ".crantcli", "credentials")
	writeEncodedToken(t, path, "loose-token")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if got := ReadStoredToken(); got != "loose-token" {
		t.Fatalf("expected loose-token, got %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected permissions tightened to 0600, got 0o%o", perm)
	}
}
