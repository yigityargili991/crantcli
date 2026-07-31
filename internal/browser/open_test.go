package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCappedOpenerOutput(t *testing.T) {
	var output cappedOutput
	if n, err := output.Write([]byte("prefix")); err != nil || n != len("prefix") {
		t.Fatalf("small Write = (%d, %v)", n, err)
	}
	payload := make([]byte, maxOpenerOutput+1024)
	if n, err := output.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(payload))
	}
	if output.Len() != maxOpenerOutput {
		t.Fatalf("buffer length = %d, want %d", output.Len(), maxOpenerOutput)
	}
}

func TestOpenURLRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "not a URL", "file:///tmp/state", "javascript:alert(1)"} {
		if _, err := OpenURLWithResult(value); err == nil {
			t.Errorf("OpenURLWithResult(%q) unexpectedly succeeded", value)
		}
	}
	if err := OpenURL(""); err == nil {
		t.Error("OpenURL(\"\") unexpectedly succeeded")
	}
}

func TestRunOpenCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper scripts use a POSIX shell")
	}
	dir := t.TempDir()
	writeScript := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(dir, strings.ReplaceAll(t.Name(), "/", "-"))
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("success", func(t *testing.T) {
		result, err := runOpenCommand(BackendGIO, writeScript(t, "exit 0"))
		if err != nil {
			t.Fatal(err)
		}
		if result.Backend != BackendGIO {
			t.Fatalf("backend = %q, want %q", result.Backend, BackendGIO)
		}
	})

	t.Run("error includes output", func(t *testing.T) {
		_, err := runOpenCommand(BackendGIO, writeScript(t, "printf 'desktop unavailable' >&2\nexit 7"))
		if err == nil || !strings.Contains(err.Error(), "desktop unavailable") {
			t.Fatalf("error = %v, want command output", err)
		}
	})

	t.Run("error truncates noisy output", func(t *testing.T) {
		noise := strings.Repeat("x", 5000)
		_, err := runOpenCommand(BackendGIO, writeScript(t, "printf '"+noise+"' >&2\nexit 8"))
		if err == nil || !strings.Contains(err.Error(), "…") || strings.Contains(err.Error(), noise) {
			t.Fatalf("error was not truncated: %v", err)
		}
	})

	t.Run("error without output", func(t *testing.T) {
		_, err := runOpenCommand(BackendGIO, writeScript(t, "exit 9"))
		if err == nil || strings.Contains(err.Error(), "desktop unavailable") {
			t.Fatalf("error = %v", err)
		}
	})
}
