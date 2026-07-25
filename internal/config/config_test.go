package config

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestStoreEncodedTokenRejectsEmptyPath(t *testing.T) {
	if err := storeEncodedToken("", "token"); err == nil {
		t.Fatal("storeEncodedToken() error = nil, want home-directory error")
	}
}
