package clipboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLimitWriterEnforcesMaximum(t *testing.T) {
	writer := &limitWriter{max: 5}

	if n, err := writer.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first Write() = (%d, %v), want (3, nil)", n, err)
	}
	if n, err := writer.Write([]byte("de")); err != nil || n != 2 {
		t.Fatalf("second Write() = (%d, %v), want (2, nil)", n, err)
	}
	if got := writer.buf.String(); got != "abcde" {
		t.Fatalf("buffer = %q, want abcde", got)
	}

	if n, err := writer.Write([]byte("f")); err == nil || n != 0 {
		t.Fatalf("overflow Write() = (%d, %v), want (0, error)", n, err)
	}
	if got := writer.buf.String(); got != "abcde" {
		t.Fatalf("overflow changed buffer to %q", got)
	}
}

func TestFindAvailableRespectsEnvironmentGateAndOrder(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing-tool")
	const displayEnv = "CRANTCLI_TEST_DISPLAY"
	t.Setenv(displayEnv, "")

	tools := []clipTool{
		{name: executable, env: displayEnv},
		{name: executable},
	}
	if got := findAvailable(tools); got != &tools[1] {
		t.Fatalf("findAvailable() = %#v, want ungated second tool", got)
	}

	t.Setenv(displayEnv, "available")
	if got := findAvailable(tools); got != &tools[0] {
		t.Fatalf("findAvailable() = %#v, want first tool", got)
	}

	if got := findAvailable([]clipTool{{name: missing}}); got != nil {
		t.Fatalf("findAvailable() = %#v, want nil for missing executable", got)
	}
}
