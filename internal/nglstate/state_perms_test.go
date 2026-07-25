package nglstate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteToFilePerm0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}

	path := filepath.Join(t.TempDir(), "out.json")
	state := map[string]interface{}{"layers": []interface{}{}}

	if err := writeToFile(state, path); err != nil {
		t.Fatalf("writeToFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("output file permissions = 0o%o, want 0600", perm)
	}
}

func TestLoadState_StdinCapDocumented(t *testing.T) {
	// The stdin bound is a constant guard; verify it stays generous enough for
	// real scenes (MiB) while remaining finite.
	if maxStateBytes < 1<<20 || maxStateBytes > 1<<30 {
		t.Fatalf("maxStateBytes = %d, want a sane 1 MiB..1 GiB window", maxStateBytes)
	}
}
