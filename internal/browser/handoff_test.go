package browser

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

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
	if info.Mode().Perm() != 0o600 {
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

func TestFileURL(t *testing.T) {
	for _, test := range []struct{ path, want string }{
		{"/home/u/.cache/crantcli/a.html", "file:///home/u/.cache/crantcli/a.html"},
		{"/tmp/dir with space/a.html", "file:///tmp/dir%20with%20space/a.html"},
	} {
		if got := fileURL(test.path); got != test.want {
			t.Errorf("fileURL(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}
