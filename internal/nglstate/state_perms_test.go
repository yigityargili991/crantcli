// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package nglstate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestWriteStateFilesRestrictExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}

	// OpenFile applies its mode only when it creates a file, so an existing
	// 0644 file must be restricted explicitly.
	t.Run("output file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.json")
		if err := os.WriteFile(path, []byte(strings.Repeat("old ", 64)+"\n"), 0o600); err != nil {
			t.Fatalf("create existing output: %v", err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("set existing output mode: %v", err)
		}

		if err := writeToFile(map[string]interface{}{"layers": []interface{}{}}, path); err != nil {
			t.Fatalf("writeToFile: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat output: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("output permissions = 0o%o, want 0600", perm)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if got, want := string(data), "{\n  \"layers\": []\n}\n"; got != want {
			t.Fatalf("output = %q, want %q with the longer old contents truncated", got, want)
		}
	})

	t.Run("default state file", func(t *testing.T) {
		previousCachePath := testCachePath
		testCachePath = t.TempDir()
		t.Cleanup(func() {
			testCachePath = previousCachePath
		})

		path := defaultStateFilePath()
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("create existing default state: %v", err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("set existing default state mode: %v", err)
		}

		if err := WriteDefaultState([]byte(`{"layers":[]}`)); err != nil {
			t.Fatalf("WriteDefaultState: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat default state: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("default state permissions = 0o%o, want 0600", perm)
		}
	})
}

func TestWriteToFileWritesThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}

	// Dotfile managers link config files into a repository: the target is
	// updated and restricted, and the link stays in place.
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatalf("set target mode: %v", err)
	}
	link := filepath.Join(dir, "out.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if err := writeToFile(map[string]interface{}{"zoom": float64(4)}, link); err != nil {
		t.Fatalf("writeToFile: %v", err)
	}

	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("writeToFile replaced the symlink with a regular file")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !strings.Contains(string(data), `"zoom": 4`) {
		t.Fatalf("target = %q, want the written state", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("target permissions = 0o%o, want 0600", perm)
	}
}

func TestWriteToFileWritesToDevice(t *testing.T) {
	// --output /dev/null (NUL on Windows) writes to the device in place.
	if err := writeToFile(map[string]interface{}{"layers": []interface{}{}}, os.DevNull); err != nil {
		t.Fatalf("writeToFile(%q): %v", os.DevNull, err)
	}
}

func TestLoadState_StdinCapDocumented(t *testing.T) {
	// The stdin bound is a constant guard; verify it stays generous enough for
	// real scenes (MiB) while remaining finite.
	if maxStateBytes < 1<<20 || maxStateBytes > 1<<30 {
		t.Fatalf("maxStateBytes = %d, want a sane 1 MiB..1 GiB window", maxStateBytes)
	}
}
