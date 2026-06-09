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

func TestGetAPITokenPrecedence(t *testing.T) {
	home, cleanup := setupTestHome(t)
	defer cleanup()
	t.Setenv("CRANTTABLE_TOKEN", "")
	t.Setenv("CRANTTABLE_TOKEN_FILE", "")

	storedPath := filepath.Join(home, ".crantcli", "credentials")
	writeEncodedToken(t, storedPath, "stored-token")
	tokenFile := filepath.Join(t.TempDir(), "seatable-token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	if got := GetAPIToken(); got != "stored-token" {
		t.Fatalf("GetAPIToken with stored only = %q, want stored-token", got)
	}
	t.Setenv("CRANTTABLE_TOKEN_FILE", tokenFile)
	if got := GetAPIToken(); got != "file-token" {
		t.Fatalf("GetAPIToken with token file = %q, want file-token", got)
	}
	t.Setenv("CRANTTABLE_TOKEN", "env-token")
	if got := GetAPIToken(); got != "env-token" {
		t.Fatalf("GetAPIToken with env = %q, want env-token", got)
	}
}

func TestGetCAVETokenPrecedence(t *testing.T) {
	home, cleanup := setupTestHome(t)
	defer cleanup()
	t.Setenv("CAVE_TOKEN", "")
	t.Setenv("CAVE_TOKEN_FILE", "")

	writeEncodedToken(t, filepath.Join(home, ".crantcli", "cave_credentials"), "stored-cave")
	tokenFile := filepath.Join(t.TempDir(), "cave-token")
	if err := os.WriteFile(tokenFile, []byte("file-cave\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	if got := GetCAVEToken(); got != "stored-cave" {
		t.Fatalf("GetCAVEToken with stored only = %q, want stored-cave", got)
	}
	t.Setenv("CAVE_TOKEN_FILE", tokenFile)
	if got := GetCAVEToken(); got != "file-cave" {
		t.Fatalf("GetCAVEToken with token file = %q, want file-cave", got)
	}
	t.Setenv("CAVE_TOKEN", "env-cave")
	if got := GetCAVEToken(); got != "env-cave" {
		t.Fatalf("GetCAVEToken with env = %q, want env-cave", got)
	}
}
