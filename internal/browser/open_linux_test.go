//go:build linux

package browser

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func isolateLinuxOpeners(t *testing.T) {
	t.Helper()
	previousPortal := openViaPortal
	previousPrepare := prepareCommandOpenURL
	previousCommand := runPlatformCommand
	t.Cleanup(func() {
		openViaPortal = previousPortal
		prepareCommandOpenURL = previousPrepare
		runPlatformCommand = previousCommand
	})
}

func TestPlatformOpenPrefersPortal(t *testing.T) {
	isolateLinuxOpeners(t)
	commands := 0
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{Backend: BackendXDGPortal}, nil
	}
	runPlatformCommand = func(Backend, string, ...string) (OpenResult, error) {
		commands++
		return OpenResult{}, errors.New("unexpected command")
	}

	result, err := platformOpenURL("https://example.org/")
	if err != nil {
		t.Fatal(err)
	}
	if result.Backend != BackendXDGPortal || commands != 0 {
		t.Fatalf("result=%#v commands=%d", result, commands)
	}
}

func TestPlatformOpenFallsBackInOrder(t *testing.T) {
	isolateLinuxOpeners(t)
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{}, errors.New("portal unavailable")
	}
	var calls []string
	runPlatformCommand = func(backend Backend, name string, args ...string) (OpenResult, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "xdg-open" {
			return OpenResult{}, errors.New("xdg failed")
		}
		return OpenResult{Backend: backend}, nil
	}

	result, err := platformOpenURL("https://example.org/state")
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"xdg-open https://example.org/state",
		"gio open https://example.org/state",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
	if result.Backend != BackendGIO {
		t.Fatalf("backend = %q, want %q", result.Backend, BackendGIO)
	}
}

func TestPlatformOpenReportsEveryFailure(t *testing.T) {
	isolateLinuxOpeners(t)
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{}, errors.New("portal failed")
	}
	runPlatformCommand = func(_ Backend, name string, _ ...string) (OpenResult, error) {
		return OpenResult{}, errors.New(name + " failed")
	}

	_, err := platformOpenURL("https://example.org/")
	if err == nil {
		t.Fatal("platformOpenURL unexpectedly succeeded")
	}
	for _, want := range []string{"portal failed", "xdg-open failed", "gio failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestOversizedURLUsesPrivateFileForCommandFallback(t *testing.T) {
	isolateLinuxOpeners(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{}, errors.New("portal unavailable")
	}
	longURL := "https://example.org/#!" + strings.Repeat("a", maxSafeOpenArgument)
	var openedTarget string
	runPlatformCommand = func(backend Backend, _ string, args ...string) (OpenResult, error) {
		openedTarget = args[len(args)-1]
		return OpenResult{Backend: backend}, nil
	}

	if _, err := platformOpenURL(longURL); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(openedTarget)
	if err != nil || parsed.Scheme != "file" {
		t.Fatalf("command target = %q, want file URL", openedTarget)
	}
	data, err := os.ReadFile(parsed.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), strings.Repeat("a", 1024)) || strings.Contains(openedTarget, "#!") {
		t.Fatal("handoff did not keep oversized state out of the command argument")
	}
	if info, err := os.Stat(parsed.Path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("handoff permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestPlatformOpenSweepsStaleHandoffsOnPortalSuccess(t *testing.T) {
	isolateLinuxOpeners(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{Backend: BackendXDGPortal}, nil
	}

	// A leftover from an earlier portal outage must not linger just because the
	// portal now works and stages nothing.
	dir, err := handoffDir()
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "crantcli_stale.html")
	if err := os.WriteFile(stale, []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-handoffLifetime - time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := platformOpenURL("https://example.org/"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale handoff survived a portal open: %v", err)
	}
}

func TestPortalHandleTokenIsValidAndUnique(t *testing.T) {
	first, err := handoffToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := handoffToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "crantcli_") || strings.ContainsAny(first, "-./") {
		t.Fatalf("invalid portal tokens %q and %q", first, second)
	}
}
