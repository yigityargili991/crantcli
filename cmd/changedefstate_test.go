package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunChangeDefStateStoresJSONArgument(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	err := runChangeDefState(changeDefStateCmd, []string{`{"zoom":4,"layers":[]}`})
	if err != nil {
		t.Fatalf("runChangeDefState: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".crantcli", "default_state.json"))
	if err != nil {
		t.Fatalf("read default state: %v", err)
	}
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("stored default is not valid JSON: %v", err)
	}
	if got := state["zoom"]; got != float64(4) {
		t.Fatalf("zoom = %#v, want 4", got)
	}
}

func TestRunChangeDefStateReadsFileAndResets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	inputPath := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(inputPath, []byte(`{"from":"file"}`), 0o600); err != nil {
		t.Fatalf("write input state: %v", err)
	}
	if err := runChangeDefState(changeDefStateCmd, []string{inputPath}); err != nil {
		t.Fatalf("store file state: %v", err)
	}

	defaultPath := filepath.Join(home, ".crantcli", "default_state.json")
	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatalf("read default state: %v", err)
	}
	if !strings.Contains(string(data), `"from": "file"`) {
		t.Fatalf("stored default = %q, want file state", data)
	}

	changeDefStateReset = true
	if err := runChangeDefState(changeDefStateCmd, nil); err != nil {
		t.Fatalf("reset default state: %v", err)
	}
	if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
		t.Fatalf("default state still exists after reset: %v", err)
	}
}

func TestRunChangeDefStateValidatesInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	if err := runChangeDefState(changeDefStateCmd, nil); err == nil {
		t.Fatal("missing input error = nil")
	}
	if err := runChangeDefState(changeDefStateCmd, []string{"{invalid"}); err == nil {
		t.Fatal("invalid JSON error = nil")
	}

	changeDefStateShow = true
	if err := runChangeDefState(changeDefStateCmd, []string{`{"layers":[]}`}); err == nil {
		t.Fatal("--show with JSON error = nil")
	}
}

func TestRunChangeDefStateRejectsNull(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	if err := runChangeDefState(changeDefStateCmd, []string{`{"zoom":4,"layers":[]}`}); err != nil {
		t.Fatalf("seed default state: %v", err)
	}
	defaultPath := filepath.Join(home, ".crantcli", "default_state.json")
	before, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatalf("read seeded default state: %v", err)
	}

	// JSON null unmarshals into a nil map without an error.
	inputPath := filepath.Join(t.TempDir(), "null.json")
	if err := os.WriteFile(inputPath, []byte("null\n"), 0o600); err != nil {
		t.Fatalf("write null input: %v", err)
	}
	if err := runChangeDefState(changeDefStateCmd, []string{inputPath}); err == nil {
		t.Fatal("runChangeDefState accepted JSON null")
	}

	after, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatalf("read default state after rejection: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("default state = %q after rejecting null, want unchanged %q", after, before)
	}
}

func TestRunChangeDefStateAcceptsEmptyObject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	if err := runChangeDefState(changeDefStateCmd, []string{`{}`}); err != nil {
		t.Fatalf("runChangeDefState rejected an empty object: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".crantcli", "default_state.json"))
	if err != nil {
		t.Fatalf("read default state: %v", err)
	}
	if string(data) != "{}\n" {
		t.Fatalf("stored default = %q, want an empty object", data)
	}
}

func resetChangeDefStateFlags(t *testing.T) {
	t.Helper()

	previousShow := changeDefStateShow
	previousReset := changeDefStateReset
	changeDefStateShow = false
	changeDefStateReset = false
	t.Cleanup(func() {
		changeDefStateShow = previousShow
		changeDefStateReset = previousReset
	})
}
