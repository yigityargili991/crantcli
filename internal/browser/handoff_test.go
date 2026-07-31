package browser

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"os/user"
)

type fakeHandoffFile struct {
	writeErr error
	syncErr  error
	closeErr error
}

func (f *fakeHandoffFile) WriteString(value string) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(value), nil
}

func (f *fakeHandoffFile) Sync() error  { return f.syncErr }
func (f *fakeHandoffFile) Close() error { return f.closeErr }

func TestCommandOpenURLPassesShortURLsThrough(t *testing.T) {
	short := "https://example.org/#!%7B%7D"
	target, err := commandOpenURL(short)
	if err != nil {
		t.Fatal(err)
	}
	if target != short {
		t.Fatalf("target = %q, want the URL unchanged", target)
	}
}

func TestCommandOpenURLDoesNotRequireCacheForShortURLs(t *testing.T) {
	unusableCache := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(unusableCache, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", unusableCache)
	t.Setenv("HOME", unusableCache)

	short := "https://example.org/#!%7B%7D"
	target, err := commandOpenURL(short)
	if err != nil {
		t.Fatalf("direct open unexpectedly required cache access: %v", err)
	}
	if target != short {
		t.Fatalf("target = %q, want the URL unchanged", target)
	}
}

func TestCommandOpenURLReportsUnavailableCacheForStagedURLs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows resolves its cache outside HOME and XDG_CACHE_HOME")
	}
	unusableCache := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(unusableCache, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", unusableCache)
	t.Setenv("HOME", unusableCache)

	long := "https://example.org/#!" + strings.Repeat("a", maxSafeOpenArgument)
	if _, err := commandOpenURL(long); err == nil {
		t.Fatal("staged open unexpectedly succeeded with an unusable cache")
	}
}

func TestCommandOpenURLReportsTokenAndFileCreationFailures(t *testing.T) {
	previousToken := generateHandoffToken
	t.Cleanup(func() { generateHandoffToken = previousToken })
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	long := "https://example.org/#!" + strings.Repeat("a", maxSafeOpenArgument)

	generateHandoffToken = func() (string, error) {
		return "", os.ErrPermission
	}
	if _, err := commandOpenURL(long); err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("token error = %v", err)
	}

	generateHandoffToken = func() (string, error) {
		return "crantcli_existing", nil
	}
	dir, err := handoffDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "crantcli_existing.html"), []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := commandOpenURL(long); err == nil || !strings.Contains(err.Error(), "creating browser handoff") {
		t.Fatalf("file creation error = %v", err)
	}
}

func TestCommandOpenURLReportsHandoffFileFailures(t *testing.T) {
	previousToken := generateHandoffToken
	previousOpen := openBrowserHandoffFile
	t.Cleanup(func() {
		generateHandoffToken = previousToken
		openBrowserHandoffFile = previousOpen
	})
	generateHandoffToken = func() (string, error) { return "crantcli_test", nil }
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	long := "https://example.org/#!" + strings.Repeat("a", maxSafeOpenArgument)

	for _, test := range []struct {
		name string
		file *fakeHandoffFile
		want string
	}{
		{name: "write", file: &fakeHandoffFile{writeErr: errors.New("disk full")}, want: "writing browser handoff"},
		{name: "sync", file: &fakeHandoffFile{syncErr: errors.New("sync failed")}, want: "syncing browser handoff"},
		{name: "close", file: &fakeHandoffFile{closeErr: errors.New("close failed")}, want: "closing browser handoff"},
	} {
		t.Run(test.name, func(t *testing.T) {
			openBrowserHandoffFile = func(string) (browserHandoffFile, error) {
				return test.file, nil
			}
			if _, err := commandOpenURL(long); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

// A state URL carries literal quotes, which Windows escapes into the URL the
// browser finally sees. Such a URL must never reach the opener as an argument,
// however short it is.
func TestCommandOpenURLStagesURLsTheOpenerCannotCarry(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	quoted := "https://example.org/#!{\"layers\":[]}"

	target, err := commandOpenURL(quoted)
	if err != nil {
		t.Fatal(err)
	}
	if openerArgumentIsSafe(quoted) {
		if target != quoted {
			t.Fatalf("target = %q, want the URL unchanged", target)
		}
		return
	}
	if !strings.HasPrefix(target, "file://") || strings.ContainsAny(target, "\"\\ \t") {
		t.Fatalf("target = %q, want a file URL the command line carries intact", target)
	}
	data, err := os.ReadFile(handoffPath(t, target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\\\"layers\\\"") {
		t.Fatalf("staged file does not hold the state URL: %s", data)
	}
}

// handoffPath turns a staged file URL back into a local path, undoing the
// slash form and percent escaping fileURL applies.
func handoffPath(t *testing.T, target string) string {
	t.Helper()
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parsing staged target %q: %v", target, err)
	}
	if runtime.GOOS == "windows" {
		return filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/"))
	}
	return parsed.Path
}

func TestCommandOpenURLStagesOversizedURLs(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	long := "https://example.org/#!" + strings.Repeat("a", maxSafeOpenArgument)

	target, err := commandOpenURL(long)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(target, "file://") || strings.Contains(target, "#!") {
		t.Fatalf("target = %q, want a file URL with no inline state", target)
	}
	path := handoffPath(t, target)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), strings.Repeat("a", 1024)) {
		t.Fatal("staged file does not contain the state URL")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestRemoveStaleHandoffsSweepsOnEveryOpen(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", t.TempDir())

	dir, err := handoffDir()
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "crantcli_stale.html")
	fresh := filepath.Join(dir, "crantcli_fresh.html")
	for _, path := range []string{stale, fresh} {
		if err := os.WriteFile(path, []byte("<!doctype html>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-handoffLifetime - time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	// A short URL needs no staging, but must still sweep.
	if _, err := commandOpenURL("https://example.org/"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale handoff survived: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh handoff was removed: %v", err)
	}
}

func TestRemoveStaleHandoffsIgnoresOtherEntries(t *testing.T) {
	dir := t.TempDir()
	keep := []string{
		filepath.Join(dir, "notes.txt"),
		filepath.Join(dir, "nested.html"),
	}
	if err := os.WriteFile(keep[0], []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(keep[1], 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-handoffLifetime - time.Hour)
	if err := os.Chtimes(keep[0], old, old); err != nil {
		t.Fatal(err)
	}

	removeStaleHandoffs(dir)
	for _, path := range keep {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was removed: %v", path, err)
		}
	}

	regularFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(regularFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	removeStaleHandoffs(regularFile)
}

func TestHandoffDirRejectsNonDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows resolves its cache outside HOME and XDG_CACHE_HOME")
	}
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cacheRoot, "crantcli", "browser-handoffs")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := handoffDir(); err == nil || !strings.Contains(err.Error(), "not a private directory") {
		t.Fatalf("handoffDir error = %v", err)
	}
}

func TestHandoffDirFallbackAndFailures(t *testing.T) {
	previousCache := handoffUserCacheDir
	previousCurrentUser := handoffCurrentUser
	previousHomeDir := handoffUserHomeDir
	previousTemp := handoffTempDir
	previousMkdir := makeHandoffDirs
	previousChmod := secureHandoffDir
	t.Cleanup(func() {
		handoffUserCacheDir = previousCache
		handoffCurrentUser = previousCurrentUser
		handoffUserHomeDir = previousHomeDir
		handoffTempDir = previousTemp
		makeHandoffDirs = previousMkdir
		secureHandoffDir = previousChmod
	})

	t.Run("stable temporary fallback", func(t *testing.T) {
		tempRoot := t.TempDir()
		handoffUserCacheDir = func() (string, error) { return "", errors.New("no cache") }
		handoffCurrentUser = func() (*user.User, error) {
			return &user.User{Uid: "1001", Username: "tester", HomeDir: "/home/tester"}, nil
		}
		handoffTempDir = func() string { return tempRoot }
		first, err := handoffDir()
		if err != nil {
			t.Fatal(err)
		}
		second, err := handoffDir()
		if err != nil {
			t.Fatal(err)
		}
		if first != second || !strings.HasPrefix(first, filepath.Join(tempRoot, "crantcli-browser-handoffs-")) {
			t.Fatalf("fallback directories = %q and %q", first, second)
		}
		stale := filepath.Join(first, "crantcli_stale.html")
		if err := os.WriteFile(stale, []byte("state"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-handoffLifetime - time.Hour)
		if err := os.Chtimes(stale, old, old); err != nil {
			t.Fatal(err)
		}
		sweepStaleHandoffs()
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatalf("stale fallback handoff survived: %v", err)
		}
	})

	t.Run("temporary fallback requires user identity", func(t *testing.T) {
		handoffUserCacheDir = func() (string, error) { return "", errors.New("no cache") }
		handoffCurrentUser = func() (*user.User, error) { return nil, errors.New("no user") }
		handoffUserHomeDir = func() (string, error) { return "", errors.New("no home") }
		if _, err := handoffDir(); err == nil || !strings.Contains(err.Error(), "determining current user") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("mkdir failure", func(t *testing.T) {
		handoffUserCacheDir = func() (string, error) { return t.TempDir(), nil }
		handoffTempDir = previousTemp
		makeHandoffDirs = func(string, os.FileMode) error { return errors.New("mkdir denied") }
		if _, err := handoffDir(); err == nil || !strings.Contains(err.Error(), "creating browser handoff directory") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("chmod failure", func(t *testing.T) {
		handoffUserCacheDir = func() (string, error) { return t.TempDir(), nil }
		makeHandoffDirs = previousMkdir
		secureHandoffDir = func(string, os.FileMode) error { return errors.New("chmod denied") }
		if _, err := handoffDir(); err == nil || !strings.Contains(err.Error(), "securing browser handoff directory") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestHandoffOwnerKey(t *testing.T) {
	previousCurrentUser := handoffCurrentUser
	previousHomeDir := handoffUserHomeDir
	t.Cleanup(func() {
		handoffCurrentUser = previousCurrentUser
		handoffUserHomeDir = previousHomeDir
	})

	handoffCurrentUser = func() (*user.User, error) {
		return &user.User{Uid: "1001", Username: "first", HomeDir: "/home/first"}, nil
	}
	first, err := handoffOwnerKey()
	if err != nil {
		t.Fatal(err)
	}
	handoffCurrentUser = func() (*user.User, error) {
		return &user.User{Uid: "1002", Username: "second", HomeDir: "/home/second"}, nil
	}
	second, err := handoffOwnerKey()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) != 24 || len(second) != 24 {
		t.Fatalf("owner keys = %q and %q", first, second)
	}

	handoffCurrentUser = func() (*user.User, error) {
		return nil, errors.New("lookup failed")
	}
	handoffUserHomeDir = func() (string, error) { return "/fallback/home", nil }
	if fallback, err := handoffOwnerKey(); err != nil || fallback == "" {
		t.Fatalf("fallback key = %q, error = %v", fallback, err)
	}

	handoffUserHomeDir = func() (string, error) { return "", errors.New("no home") }
	if _, err := handoffOwnerKey(); err == nil {
		t.Fatal("handoffOwnerKey unexpectedly accepted an unknown user")
	}
}

func TestHandoffTokenReportsRandomFailure(t *testing.T) {
	previousRead := handoffRandomRead
	handoffRandomRead = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	t.Cleanup(func() { handoffRandomRead = previousRead })
	if _, err := handoffToken(); err == nil || !strings.Contains(err.Error(), "entropy unavailable") {
		t.Fatalf("error = %v", err)
	}
}

func TestFileURL(t *testing.T) {
	for _, test := range []struct{ path, want string }{
		{"/home/u/.cache/crantcli/a.html", "file:///home/u/.cache/crantcli/a.html"},
		{"/tmp/dir with space/a.html", "file:///tmp/dir%20with%20space/a.html"},
		{"relative/path.html", "file:///relative/path.html"},
	} {
		if got := fileURL(test.path); got != test.want {
			t.Errorf("fileURL(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}
