package config

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

type fakeCredentialKeyring struct {
	values     map[string]string
	getErr     error
	setErr     error
	deleteErr  error
	discardSet bool
}

func newFakeCredentialKeyring() *fakeCredentialKeyring {
	return &fakeCredentialKeyring{values: make(map[string]string)}
}

func credentialKey(service, account string) string {
	return service + "\x00" + account
}

func (f *fakeCredentialKeyring) Get(service, account string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	token, ok := f.values[credentialKey(service, account)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return token, nil
}

func (f *fakeCredentialKeyring) Set(service, account, token string) error {
	if f.setErr != nil {
		return f.setErr
	}
	if !f.discardSet {
		f.values[credentialKey(service, account)] = token
	}
	return nil
}

func (f *fakeCredentialKeyring) Delete(service, account string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	key := credentialKey(service, account)
	if _, ok := f.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.values, key)
	return nil
}

func setupTestHome(t *testing.T) (string, *fakeCredentialKeyring) {
	t.Helper()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	backend := newFakeCredentialKeyring()
	originalBackend := activeCredentialKeyring
	originalFallback := credentialFileFallbackAllowed
	activeCredentialKeyring = backend
	credentialFileFallbackAllowed = false
	t.Cleanup(func() {
		activeCredentialKeyring = originalBackend
		credentialFileFallbackAllowed = originalFallback
	})
	return tempHome, backend
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

func TestCredentialFileFallbackPolicy(t *testing.T) {
	want := runtime.GOOS == "linux"
	if credentialFileFallbackAllowed != want {
		t.Fatalf("credentialFileFallbackAllowed = %v, want %v on %s", credentialFileFallbackAllowed, want, runtime.GOOS)
	}
}

func TestReadStoredToken(t *testing.T) {
	t.Run("reads new config path", func(t *testing.T) {
		home, backend := setupTestHome(t)

		path := filepath.Join(home, ".crantcli", "credentials")
		writeEncodedToken(t, path, "new-token")

		if got := ReadStoredToken(); got != "new-token" {
			t.Fatalf("expected new-token, got %q", got)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("migrated credential file still exists: %v", err)
		}
		if got := backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)]; got != "new-token" {
			t.Fatalf("migrated keyring token = %q, want new-token", got)
		}
	})

	t.Run("falls back to legacy config path", func(t *testing.T) {
		home, _ := setupTestHome(t)

		path := filepath.Join(home, ".crant_type_look", "credentials")
		writeEncodedToken(t, path, "legacy-token")

		if got := ReadStoredToken(); got != "legacy-token" {
			t.Fatalf("expected legacy-token, got %q", got)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy credential file still exists after migration: %v", err)
		}
	})

	t.Run("prefers new config path over legacy", func(t *testing.T) {
		home, _ := setupTestHome(t)

		writeEncodedToken(t, filepath.Join(home, ".crantcli", "credentials"), "new-token")
		writeEncodedToken(t, filepath.Join(home, ".crant_type_look", "credentials"), "legacy-token")

		if got := ReadStoredToken(); got != "new-token" {
			t.Fatalf("expected new-token, got %q", got)
		}
	})
}

func TestReadStoredToken_TightensLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows credentials never use the file fallback")
	}
	home, backend := setupTestHome(t)
	backend.setErr = errors.New("keyring unavailable")
	credentialFileFallbackAllowed = true

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

func TestStoreTokenRoundTrip(t *testing.T) {
	home, backend := setupTestHome(t)

	if err := StoreToken("secret-token"); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	path := filepath.Join(home, ".crantcli", "credentials")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credential unexpectedly written to fallback file: %v", err)
	}
	if got := backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)]; got != "secret-token" {
		t.Fatalf("keyring token = %q, want secret-token", got)
	}
	if got := ReadStoredToken(); got != "secret-token" {
		t.Fatalf("ReadStoredToken() = %q, want %q", got, "secret-token")
	}
}

func TestReadStoredToken_RemovesObsoleteFileWhenKeyringAlreadyHasToken(t *testing.T) {
	home, backend := setupTestHome(t)
	path := filepath.Join(home, ".crantcli", "credentials")
	writeEncodedToken(t, path, "keyring-token")
	backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)] = "keyring-token"

	if got := ReadStoredToken(); got != "keyring-token" {
		t.Fatalf("ReadStoredToken() = %q, want keyring-token", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete credential file still exists: %v", err)
	}
}

func TestReadStoredToken_RetainsDifferentLegacyFile(t *testing.T) {
	home, backend := setupTestHome(t)
	path := filepath.Join(home, ".crantcli", "credentials")
	writeEncodedToken(t, path, "different-file-token")
	backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)] = "keyring-token"

	if got := ReadStoredToken(); got != "keyring-token" {
		t.Fatalf("ReadStoredToken() = %q, want keyring-token", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("different legacy credential file was removed: %v", err)
	}
}

func TestReadStoredToken_LinuxFallbackReplacesStaleKeyringToken(t *testing.T) {
	home, backend := setupTestHome(t)
	credentialFileFallbackAllowed = true
	path := filepath.Join(home, ".crantcli", "credentials")
	writeEncodedToken(t, path, "new-fallback-token")
	backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)] = "stale-keyring-token"

	if got := ReadStoredToken(); got != "new-fallback-token" {
		t.Fatalf("ReadStoredToken() = %q, want new-fallback-token", got)
	}
	if got := backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)]; got != "new-fallback-token" {
		t.Fatalf("keyring token = %q, want migrated fallback token", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fallback file still exists after verified migration: %v", err)
	}
}

func TestReadStoredToken_LinuxFallbackReadsLegacyFileAfterKeyring(t *testing.T) {
	tests := []struct {
		name         string
		keyringToken string
		keyringErr   error
		wantToken    string
		wantKeyring  string
		wantLegacy   bool
		wantFallback bool
	}{
		{
			name:         "keyring token wins over a differing legacy file",
			keyringToken: "keyring-token",
			wantToken:    "keyring-token",
			wantKeyring:  "keyring-token",
			wantLegacy:   true,
		},
		{
			name:         "keyring token leaves a matching legacy file alone",
			keyringToken: "legacy-token",
			wantToken:    "legacy-token",
			wantKeyring:  "legacy-token",
			wantLegacy:   true,
		},
		{
			name:        "legacy file migrates into an empty keyring",
			wantToken:   "legacy-token",
			wantKeyring: "legacy-token",
		},
		{
			name:         "legacy file moves to the fallback file without a keyring",
			keyringErr:   errors.New("keyring unavailable"),
			wantToken:    "legacy-token",
			wantFallback: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, backend := setupTestHome(t)
			credentialFileFallbackAllowed = true
			backend.getErr = tt.keyringErr
			backend.setErr = tt.keyringErr
			if tt.keyringToken != "" {
				backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)] = tt.keyringToken
			}
			legacyPath := filepath.Join(home, ".crantinject", "credentials")
			fallbackPath := filepath.Join(home, ".crantcli", "credentials")
			writeEncodedToken(t, legacyPath, "legacy-token")

			if got := ReadStoredToken(); got != tt.wantToken {
				t.Fatalf("ReadStoredToken() = %q, want %q", got, tt.wantToken)
			}
			if got := backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)]; got != tt.wantKeyring {
				t.Fatalf("keyring token = %q, want %q", got, tt.wantKeyring)
			}
			if _, err := os.Stat(legacyPath); (err == nil) != tt.wantLegacy {
				t.Fatalf("legacy credential file exists = %v, want %v", err == nil, tt.wantLegacy)
			}
			if tt.wantFallback {
				if got, err := readEncodedTokenAtPath(fallbackPath); err != nil || got != tt.wantToken {
					t.Fatalf("fallback file token = %q (error %v), want %q", got, err, tt.wantToken)
				}
			} else if _, err := os.Stat(fallbackPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("fallback file unexpectedly exists: %v", err)
			}
		})
	}
}

func TestReadStoredToken_KeepsFileWhenKeyringRoundTripFails(t *testing.T) {
	home, backend := setupTestHome(t)
	backend.discardSet = true
	path := filepath.Join(home, ".crantcli", "credentials")
	writeEncodedToken(t, path, "legacy-token")

	if got := ReadStoredToken(); got != "" {
		t.Fatalf("ReadStoredToken() = %q, want fail-closed empty result", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("credential file removed before keyring verification: %v", err)
	}
}

func TestStoreToken_LinuxFileFallbackIsPrivateAndAtomic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows credentials never use the file fallback")
	}
	home, backend := setupTestHome(t)
	backend.setErr = errors.New("keyring unavailable")
	credentialFileFallbackAllowed = true

	dir := filepath.Join(home, ".crantcli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "credentials")
	writeEncodedToken(t, path, "old-token")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod credential: %v", err)
	}

	if err := StoreToken("new-token"); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stored token: %v", err)
	}
	wantEncoded := base64.StdEncoding.EncodeToString([]byte("new-token")) + "\n"
	if got := string(data); got != wantEncoded {
		t.Fatalf("stored credential = %q, want base64 payload %q", got, wantEncoded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat stored token: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("stored token permissions = 0o%o, want 0600", perm)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat config directory: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("config directory permissions = 0o%o, want 0700", perm)
	}
	if got := ReadStoredToken(); got != "new-token" {
		t.Fatalf("ReadStoredToken() from fallback = %q, want new-token", got)
	}
}

func TestStoreToken_FailsClosedWithoutSystemKeyring(t *testing.T) {
	home, backend := setupTestHome(t)
	backend.setErr = errors.New("keyring unavailable")
	credentialFileFallbackAllowed = false

	err := StoreToken("must-not-reach-disk")
	if err == nil || !strings.Contains(err.Error(), "system credential store is unavailable") {
		t.Fatalf("StoreToken() error = %v, want unavailable credential store error", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".crantcli", "credentials")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("credential file exists after fail-closed store: %v", statErr)
	}
}

func TestGetAPIToken_DoesNotUseLegacyFileWithoutSecureStore(t *testing.T) {
	home, backend := setupTestHome(t)
	backend.getErr = errors.New("keyring unavailable")
	backend.setErr = errors.New("keyring unavailable")
	credentialFileFallbackAllowed = false
	writeEncodedToken(t, filepath.Join(home, ".crantcli", "credentials"), "legacy-file-token")
	t.Setenv("CRANTTABLE_TOKEN", "environment-token")

	if got := GetAPIToken(); got != "environment-token" {
		t.Fatalf("GetAPIToken() = %q, want environment-token", got)
	}
}

func TestStoreEncodedTokenRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows credentials never use the file fallback")
	}
	home, _ := setupTestHome(t)
	dir := filepath.Join(home, ".crantcli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(home, "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	path := filepath.Join(dir, "credentials")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := storeEncodedToken(path, "secret-token"); err == nil {
		t.Fatal("storeEncodedToken() error = nil, want symlink rejection")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if got := string(data); got != "unchanged" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestReadStoredToken_DoesNotRemoveThroughDirectorySymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links may require elevated Windows privileges")
	}
	home, backend := setupTestHome(t)
	backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)] = "keyring-token"

	targetDir := filepath.Join(home, "target")
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	targetFile := filepath.Join(targetDir, "credentials")
	writeEncodedToken(t, targetFile, "must-not-be-deleted")
	if err := os.Symlink(targetDir, filepath.Join(home, ".crantcli")); err != nil {
		t.Fatalf("symlink config directory: %v", err)
	}

	if got := ReadStoredToken(); got != "keyring-token" {
		t.Fatalf("ReadStoredToken() = %q, want keyring-token", got)
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Fatalf("credential target was removed through directory symlink: %v", err)
	}
}

func TestGetAPITokenPrecedence(t *testing.T) {
	t.Run("stored token wins", func(t *testing.T) {
		home, _ := setupTestHome(t)

		tokenFile := filepath.Join(home, "token-file")
		if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
			t.Fatalf("write token file: %v", err)
		}
		writeEncodedToken(t, filepath.Join(home, ".crantcli", "credentials"), "stored-token")
		t.Setenv("CRANTTABLE_TOKEN", "env-token")
		t.Setenv("CRANTTABLE_TOKEN_FILE", tokenFile)

		if got := GetAPIToken(); got != "stored-token" {
			t.Fatalf("GetAPIToken() = %q, want stored-token", got)
		}
	})

	t.Run("environment wins over token file", func(t *testing.T) {
		home, _ := setupTestHome(t)

		tokenFile := filepath.Join(home, "token-file")
		if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
			t.Fatalf("write token file: %v", err)
		}
		t.Setenv("CRANTTABLE_TOKEN", "env-token")
		t.Setenv("CRANTTABLE_TOKEN_FILE", tokenFile)

		if got := GetAPIToken(); got != "env-token" {
			t.Fatalf("GetAPIToken() = %q, want env-token", got)
		}
	})

	t.Run("token file is trimmed", func(t *testing.T) {
		home, _ := setupTestHome(t)

		tokenFile := filepath.Join(home, "token-file")
		if err := os.WriteFile(tokenFile, []byte("  file-token \n"), 0o600); err != nil {
			t.Fatalf("write token file: %v", err)
		}
		t.Setenv("CRANTTABLE_TOKEN", "")
		t.Setenv("CRANTTABLE_TOKEN_FILE", tokenFile)

		if got := GetAPIToken(); got != "file-token" {
			t.Fatalf("GetAPIToken() = %q, want file-token", got)
		}
	})
}

func TestCAVETokenSources(t *testing.T) {
	home, backend := setupTestHome(t)

	t.Setenv("CAVE_TOKEN", "env-cave-token")
	t.Setenv("CAVE_TOKEN_FILE", "")
	if got := GetCAVEToken(); got != "env-cave-token" {
		t.Fatalf("GetCAVEToken() = %q, want env-cave-token", got)
	}

	if err := StoreCAVEToken("stored-cave-token"); err != nil {
		t.Fatalf("StoreCAVEToken: %v", err)
	}
	if got := ReadStoredCAVEToken(); got != "stored-cave-token" {
		t.Fatalf("ReadStoredCAVEToken() = %q, want stored-cave-token", got)
	}
	if got := GetCAVEToken(); got != "stored-cave-token" {
		t.Fatalf("GetCAVEToken() = %q, want stored-cave-token", got)
	}

	path := filepath.Join(home, ".crantcli", "cave_credentials")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CAVE token unexpectedly written to fallback file: %v", err)
	}
	if got := backend.values[credentialKey(credentialKeyringService, caveCredentialAccount)]; got != "stored-cave-token" {
		t.Fatalf("CAVE keyring token = %q, want stored-cave-token", got)
	}
}

func TestReadStoredCAVEToken(t *testing.T) {
	t.Run("migrates legacy crantinject path", func(t *testing.T) {
		home, backend := setupTestHome(t)

		path := filepath.Join(home, ".crantinject", "cave_credentials")
		writeEncodedToken(t, path, "legacy-cave-token")

		if got := ReadStoredCAVEToken(); got != "legacy-cave-token" {
			t.Fatalf("ReadStoredCAVEToken() = %q, want legacy-cave-token", got)
		}
		if got := backend.values[credentialKey(credentialKeyringService, caveCredentialAccount)]; got != "legacy-cave-token" {
			t.Fatalf("CAVE keyring token = %q, want legacy-cave-token", got)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy CAVE credential file still exists after migration: %v", err)
		}
	})

	t.Run("prefers new config path over legacy", func(t *testing.T) {
		home, backend := setupTestHome(t)

		newPath := filepath.Join(home, ".crantcli", "cave_credentials")
		legacyPath := filepath.Join(home, ".crantinject", "cave_credentials")
		writeEncodedToken(t, newPath, "new-cave-token")
		writeEncodedToken(t, legacyPath, "legacy-cave-token")

		if got := ReadStoredCAVEToken(); got != "new-cave-token" {
			t.Fatalf("ReadStoredCAVEToken() = %q, want new-cave-token", got)
		}
		if got := backend.values[credentialKey(credentialKeyringService, caveCredentialAccount)]; got != "new-cave-token" {
			t.Fatalf("CAVE keyring token = %q, want new-cave-token", got)
		}
		for _, path := range []string{newPath, legacyPath} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("CAVE credential file %s still exists after migration: %v", path, err)
			}
		}
	})

	t.Run("does not read SeaTable credential files", func(t *testing.T) {
		home, backend := setupTestHome(t)

		var paths []string
		for _, dir := range []string{".crantcli", ".crantinject", ".crant_type_look"} {
			path := filepath.Join(home, dir, "credentials")
			writeEncodedToken(t, path, "seatable-token")
			paths = append(paths, path)
		}

		if got := ReadStoredCAVEToken(); got != "" {
			t.Fatalf("ReadStoredCAVEToken() = %q, want no CAVE token", got)
		}
		if _, ok := backend.values[credentialKey(credentialKeyringService, caveCredentialAccount)]; ok {
			t.Fatal("SeaTable credential was stored as the CAVE token")
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("SeaTable credential file %s was removed: %v", path, err)
			}
		}
	})
}

func TestReadStoredCAVEToken_RemovesLegacyFileWhenKeyringAlreadyHasToken(t *testing.T) {
	home, backend := setupTestHome(t)
	path := filepath.Join(home, ".crantinject", "cave_credentials")
	writeEncodedToken(t, path, "keyring-cave-token")
	backend.values[credentialKey(credentialKeyringService, caveCredentialAccount)] = "keyring-cave-token"

	if got := ReadStoredCAVEToken(); got != "keyring-cave-token" {
		t.Fatalf("ReadStoredCAVEToken() = %q, want keyring-cave-token", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete legacy CAVE credential file still exists: %v", err)
	}
}

func TestStoreCAVEToken_RemovesLegacyFile(t *testing.T) {
	home, backend := setupTestHome(t)
	path := filepath.Join(home, ".crantinject", "cave_credentials")
	writeEncodedToken(t, path, "old-cave-token")

	if err := StoreCAVEToken("new-cave-token"); err != nil {
		t.Fatalf("StoreCAVEToken: %v", err)
	}
	if got := backend.values[credentialKey(credentialKeyringService, caveCredentialAccount)]; got != "new-cave-token" {
		t.Fatalf("CAVE keyring token = %q, want new-cave-token", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy CAVE credential file still exists after StoreCAVEToken: %v", err)
	}
}

func TestStoreEncodedTokenRejectsEmptyPath(t *testing.T) {
	if err := storeEncodedToken("", "token"); err == nil {
		t.Fatal("storeEncodedToken() error = nil, want home-directory error")
	}
}

func writeCAVEClientToken(t *testing.T, home, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(home, ".cloudvolume", "secrets", "global.daf-apis.com-cave-secret.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write caveclient token file: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod caveclient token file: %v", err)
	}
	return path
}

// captureStderr returns what fn writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	capture, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	// Windows cannot delete an open file, so TempDir's cleanup would fail.
	defer capture.Close()
	original := os.Stderr
	os.Stderr = capture
	defer func() { os.Stderr = original }()
	fn()
	data, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	return string(data)
}

func TestLookupCAVEGlobalTokenPrecedence(t *testing.T) {
	home, backend := setupTestHome(t)
	t.Setenv("CAVE_GLOBAL_TOKEN", "")
	t.Setenv("CAVE_GLOBAL_TOKEN_FILE", "")
	sources := CAVEGlobalTokenSources()
	caveClientPath := filepath.Join(home, ".cloudvolume", "secrets", "global.daf-apis.com-cave-secret.json")
	if got, want := sources[len(sources)-1], "caveclient's token file "+caveClientPath; got != want {
		t.Fatalf("last source = %q, want %q", got, want)
	}

	expect := func(wantToken string, wantSource int) {
		t.Helper()
		token, source := LookupCAVEGlobalToken()
		if wantSource < 0 {
			if token != "" || source != "" {
				t.Fatalf("LookupCAVEGlobalToken() = %q, %q; want nothing", token, source)
			}
			return
		}
		if token != wantToken || source != sources[wantSource] {
			t.Fatalf("LookupCAVEGlobalToken() = %q, %q; want %q, %q", token, source, wantToken, sources[wantSource])
		}
	}

	expect("", -1)

	writeCAVEClientToken(t, home, `{"token": " caveclient-token\n"}`, 0o600)
	expect("caveclient-token", 3)

	// A blank CAVE_GLOBAL_TOKEN is no token either: the lookup moves on to
	// caveclient's file instead of stopping at the variable.
	t.Setenv("CAVE_GLOBAL_TOKEN", blankToken)
	expect("caveclient-token", 3)

	// An empty token file is no token: the lookup moves on to caveclient's.
	tokenFile := filepath.Join(t.TempDir(), "global-token")
	if err := os.WriteFile(tokenFile, []byte("\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("CAVE_GLOBAL_TOKEN_FILE", tokenFile)
	expect("caveclient-token", 3)

	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	expect("file-token", 2)

	t.Setenv("CAVE_GLOBAL_TOKEN", "env-token")
	expect("env-token", 1)

	if err := StoreCAVEGlobalToken("stored-token"); err != nil {
		t.Fatalf("StoreCAVEGlobalToken: %v", err)
	}
	expect("stored-token", 0)
	if got := backend.values[credentialKey(credentialKeyringService, caveGlobalCredentialAccount)]; got != "stored-token" {
		t.Fatalf("global keyring token = %q, want stored-token", got)
	}

	// The two CAVE tokens come from different auth servers and never stand
	// in for each other.
	t.Setenv("CAVE_TOKEN", "")
	t.Setenv("CAVE_TOKEN_FILE", "")
	if token, source := LookupCAVEToken(); token != "" || source != "" {
		t.Fatalf("LookupCAVEToken() = %q, %q; want nothing while only global tokens exist", token, source)
	}
}

func TestLookupCAVETokenNamesSource(t *testing.T) {
	_, _ = setupTestHome(t)
	t.Setenv("CAVE_TOKEN", "")
	tokenFile := filepath.Join(t.TempDir(), "cave-token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("CAVE_TOKEN_FILE", tokenFile)
	sources := CAVETokenSources()

	if token, source := LookupCAVEToken(); token != "file-token" || source != sources[2] {
		t.Fatalf("LookupCAVEToken() = %q, %q; want file-token from %q", token, source, sources[2])
	}
	t.Setenv("CAVE_TOKEN", "env-token")
	if token, source := LookupCAVEToken(); token != "env-token" || source != sources[1] {
		t.Fatalf("LookupCAVEToken() = %q, %q; want env-token from %q", token, source, sources[1])
	}
	if err := StoreCAVEToken("stored-token"); err != nil {
		t.Fatalf("StoreCAVEToken: %v", err)
	}
	if token, source := LookupCAVEToken(); token != "stored-token" || source != sources[0] {
		t.Fatalf("LookupCAVEToken() = %q, %q; want stored-token from %q", token, source, sources[0])
	}
	if got := GetCAVEToken(); got != "stored-token" {
		t.Fatalf("GetCAVEToken() = %q, want stored-token", got)
	}
}

// blankToken is whitespace that can stand where a token belongs, such as a
// CI variable set to a space. It is no token.
const blankToken = " \t\r\n"

func TestTokenLookupsSkipBlankValues(t *testing.T) {
	tests := []struct {
		name, envVar, fileVar, account string
		lookup                         func() (token, source string)
		// namesSource is false for GetAPIToken, which does not say where
		// its token came from.
		namesSource bool
	}{
		{"GetAPIToken", "CRANTTABLE_TOKEN", "CRANTTABLE_TOKEN_FILE", seaTableCredentialAccount,
			func() (string, string) { return GetAPIToken(), "" }, false},
		{"LookupCAVEToken", "CAVE_TOKEN", "CAVE_TOKEN_FILE", caveCredentialAccount, LookupCAVEToken, true},
		{"LookupCAVEGlobalToken", "CAVE_GLOBAL_TOKEN", "CAVE_GLOBAL_TOKEN_FILE", caveGlobalCredentialAccount,
			LookupCAVEGlobalToken, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, backend := setupTestHome(t)
			expect := func(wantToken, wantSource string) {
				t.Helper()
				if !tt.namesSource {
					wantSource = ""
				}
				if token, source := tt.lookup(); token != wantToken || source != wantSource {
					t.Errorf("%s() = %q, %q; want %q, %q", tt.name, token, source, wantToken, wantSource)
				}
			}
			tokenFile := filepath.Join(t.TempDir(), "token-file")
			writeTokenFile := func(content string) {
				t.Helper()
				if err := os.WriteFile(tokenFile, []byte(content), 0o600); err != nil {
					t.Fatalf("write token file: %v", err)
				}
			}
			t.Setenv(tt.envVar, blankToken)
			t.Setenv(tt.fileVar, tokenFile)

			// Whitespace in the variable and in the file is no token.
			writeTokenFile(blankToken)
			expect("", "")

			// A blank variable does not hide the token file.
			writeTokenFile("file-token\n")
			expect("file-token", tt.fileVar)

			// A token is used without the whitespace around it.
			t.Setenv(tt.envVar, " env-token\n")
			expect("env-token", tt.envVar)

			// A blank stored token does not hide the variable.
			key := credentialKey(credentialKeyringService, tt.account)
			backend.values[key] = blankToken
			expect("env-token", tt.envVar)

			backend.values[key] = " stored-token\n"
			expect("stored-token", storedCredentialSource)
		})
	}
}

func TestReadStoredCredentialSkipsBlankValues(t *testing.T) {
	t.Run("blank file does not hide an older one", func(t *testing.T) {
		// readStoredCredential reads the credential store first, or on
		// Linux the files first.
		for _, fallbackAllowed := range []bool{false, true} {
			home, backend := setupTestHome(t)
			credentialFileFallbackAllowed = fallbackAllowed
			writeEncodedToken(t, filepath.Join(home, ".crantcli", "credentials"), blankToken)
			writeEncodedToken(t, filepath.Join(home, ".crant_type_look", "credentials"), " legacy-token\n")

			if got := ReadStoredToken(); got != "legacy-token" {
				t.Errorf("file fallback %v: ReadStoredToken() = %q, want legacy-token", fallbackAllowed, got)
			}
			if got := backend.values[credentialKey(credentialKeyringService, seaTableCredentialAccount)]; got != "legacy-token" {
				t.Errorf("file fallback %v: migrated keyring token = %q, want legacy-token", fallbackAllowed, got)
			}
		}
	})

	t.Run("blank file is not migrated", func(t *testing.T) {
		home, backend := setupTestHome(t)
		writeEncodedToken(t, filepath.Join(home, ".crantcli", "cave_credentials"), blankToken)

		if got := ReadStoredCAVEToken(); got != "" {
			t.Errorf("ReadStoredCAVEToken() = %q, want no token", got)
		}
		if got, ok := backend.values[credentialKey(credentialKeyringService, caveCredentialAccount)]; ok {
			t.Errorf("blank credential file was migrated into the credential store as %q", got)
		}
	})

	t.Run("blank store entry gives way to the file", func(t *testing.T) {
		home, backend := setupTestHome(t)
		key := credentialKey(credentialKeyringService, caveGlobalCredentialAccount)
		backend.values[key] = blankToken
		writeEncodedToken(t, filepath.Join(home, ".crantcli", "cave_global_credentials"), "file-token")

		if got := ReadStoredCAVEGlobalToken(); got != "file-token" {
			t.Errorf("ReadStoredCAVEGlobalToken() = %q, want file-token", got)
		}
		if got := backend.values[key]; got != "file-token" {
			t.Errorf("keyring token = %q, want the migrated file-token", got)
		}
	})
}

func TestLookupCAVEGlobalTokenOnlyReadsCAVEClientFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	home, backend := setupTestHome(t)
	t.Setenv("CAVE_GLOBAL_TOKEN", "")
	t.Setenv("CAVE_GLOBAL_TOKEN_FILE", "")

	// caveclient owns this file, so even permissions crantcli would tighten
	// on its own credential files stay as they are.
	content := `{"token": "caveclient-token", "other_key": "kept"}`
	path := writeCAVEClientToken(t, home, content, 0o644)
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	for range 2 {
		if token, _ := LookupCAVEGlobalToken(); token != "caveclient-token" {
			t.Fatalf("LookupCAVEGlobalToken() = %q, want caveclient-token", token)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != content || info.Mode().Perm() != 0o644 || !info.ModTime().Equal(past) {
		t.Fatalf("caveclient token file changed: content %q, mode %o, mtime %v", data, info.Mode().Perm(), info.ModTime())
	}
	if _, ok := backend.values[credentialKey(credentialKeyringService, caveGlobalCredentialAccount)]; ok {
		t.Fatal("caveclient's token was copied into the credential store")
	}
	if _, err := os.Stat(filepath.Join(home, ".crantcli", "cave_global_credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("caveclient's token was copied into a crantcli credential file: %v", err)
	}

	// A symbolic link, say into a secrets mount, is read through.
	target := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(target, []byte(`{"token":"linked-token"}`), 0o600); err != nil {
		t.Fatalf("write link target: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if token, _ := LookupCAVEGlobalToken(); token != "linked-token" {
		t.Fatalf("LookupCAVEGlobalToken() through a link = %q, want linked-token", token)
	}
}

func TestReadCAVEClientTokenIgnoresUnusableFiles(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		name, content string
		wantWarning   string
	}{
		{"no token key", `{"middle_auth_token": "x"}`, ""},
		{"null token", `{"token": null}`, ""},
		{"truncated JSON", `{"token": "secret-value"`, "unexpected end of JSON input"},
		{"too large", `{"token": "` + strings.Repeat("x", maxCAVEClientTokenFileBytes) + `"}`, "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeCAVEClientToken(t, home, tt.content, 0o600)
			var token string
			warning := captureStderr(t, func() { token = readCAVEClientToken(path) })
			if token != "" {
				t.Fatalf("readCAVEClientToken() = %q, want no token", token)
			}
			if tt.wantWarning == "" && warning != "" {
				t.Fatalf("unexpected warning %q", warning)
			}
			if !strings.Contains(warning, tt.wantWarning) || (tt.wantWarning != "" && !strings.Contains(warning, path)) {
				t.Fatalf("warning = %q, want it to name %s and contain %q", warning, path, tt.wantWarning)
			}
			if strings.Contains(warning, "secret-value") {
				t.Fatalf("warning leaks the file's token: %q", warning)
			}
		})
	}

	t.Run("directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "global.daf-apis.com-cave-secret.json")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		var token string
		warning := captureStderr(t, func() { token = readCAVEClientToken(dir) })
		if token != "" || !strings.Contains(warning, "not a regular file") {
			t.Fatalf("readCAVEClientToken(dir) = %q with warning %q; want no token and a warning", token, warning)
		}
	})

	t.Run("missing", func(t *testing.T) {
		var token string
		warning := captureStderr(t, func() { token = readCAVEClientToken(filepath.Join(t.TempDir(), "absent.json")) })
		if token != "" || warning != "" {
			t.Fatalf("readCAVEClientToken(missing) = %q with warning %q; want neither", token, warning)
		}
	})
}
