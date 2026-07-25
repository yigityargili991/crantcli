package nglstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultStateLifecycle(t *testing.T) {
	previousCachePath := testCachePath
	testCachePath = t.TempDir()
	t.Cleanup(func() {
		testCachePath = previousCachePath
	})

	data, err := ReadDefaultState()
	if err != nil {
		t.Fatalf("ReadDefaultState before write: %v", err)
	}
	if data != nil {
		t.Fatalf("ReadDefaultState before write = %q, want nil", data)
	}

	const state = `{"layers":[]}`
	if err := WriteDefaultState([]byte(state)); err != nil {
		t.Fatalf("WriteDefaultState: %v", err)
	}
	data, err = ReadDefaultState()
	if err != nil {
		t.Fatalf("ReadDefaultState after write: %v", err)
	}
	if got := string(data); got != state {
		t.Fatalf("ReadDefaultState() = %q, want %q", got, state)
	}

	info, err := os.Stat(filepath.Join(testCachePath, "default_state.json"))
	if err != nil {
		t.Fatalf("stat default state: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("default state permissions = 0o%o, want 0600", perm)
	}

	if err := WriteDefaultState(nil); err != nil {
		t.Fatalf("remove default state: %v", err)
	}
	if err := WriteDefaultState(nil); err != nil {
		t.Fatalf("remove missing default state: %v", err)
	}
	data, err = ReadDefaultState()
	if err != nil {
		t.Fatalf("ReadDefaultState after remove: %v", err)
	}
	if data != nil {
		t.Fatalf("ReadDefaultState after remove = %q, want nil", data)
	}
}

func TestReadDefaultStateReturnsReadError(t *testing.T) {
	previousCachePath := testCachePath
	testCachePath = t.TempDir()
	t.Cleanup(func() {
		testCachePath = previousCachePath
	})

	if err := os.Mkdir(filepath.Join(testCachePath, "default_state.json"), 0o700); err != nil {
		t.Fatalf("create directory at state path: %v", err)
	}
	if _, err := ReadDefaultState(); err == nil {
		t.Fatal("ReadDefaultState() error = nil, want read error")
	}
}
