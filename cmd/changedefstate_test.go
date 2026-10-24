package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crantcli/internal/nglstate"
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

func TestRunChangeDefStateBuiltinPrintsTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	var out bytes.Buffer
	changeDefStateCmd.SetOut(&out)
	t.Cleanup(func() { changeDefStateCmd.SetOut(nil) })

	changeDefStateBuiltin = true
	if err := runChangeDefState(changeDefStateCmd, nil); err != nil {
		t.Fatalf("runChangeDefState --builtin: %v", err)
	}

	if got := out.String(); got != string(nglstate.DefaultScene) {
		t.Fatalf("--builtin output = %q, want the embedded default scene", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".crantcli", "default_state.json")); !os.IsNotExist(err) {
		t.Fatalf("--builtin touched the default state file: %v", err)
	}
}

func TestRunChangeDefStateBuiltinRejectsCombinations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	changeDefStateBuiltin = true

	changeDefStateShow = true
	if err := runChangeDefState(changeDefStateCmd, nil); err == nil {
		t.Fatal("--builtin with --show error = nil")
	}
	changeDefStateShow = false

	changeDefStateReset = true
	if err := runChangeDefState(changeDefStateCmd, nil); err == nil {
		t.Fatal("--builtin with --reset error = nil")
	}
	changeDefStateReset = false

	if err := runChangeDefState(changeDefStateCmd, []string{`{"layers":[]}`}); err == nil {
		t.Fatal("--builtin with JSON argument error = nil")
	}
	if err := runChangeDefState(changeDefStateCmd, []string{filepath.Join(t.TempDir(), "state.json")}); err == nil {
		t.Fatal("--builtin with file argument error = nil")
	}
}

func TestGenerateDeprecatedAndHidden(t *testing.T) {
	if !generateCmd.Hidden {
		t.Error("generate must be hidden from --help")
	}
	if generateCmd.Deprecated == "" {
		t.Fatal("generate must be deprecated")
	}
	for _, want := range []string{"change-def-state --builtin", "v0.24.0"} {
		if !strings.Contains(generateCmd.Deprecated, want) {
			t.Errorf("generate deprecation warning %q lacks %q", generateCmd.Deprecated, want)
		}
	}
}

func resetChangeDefStateFlags(t *testing.T) {
	t.Helper()

	previousBuiltin := changeDefStateBuiltin
	previousShow := changeDefStateShow
	previousReset := changeDefStateReset
	changeDefStateBuiltin = false
	changeDefStateShow = false
	changeDefStateReset = false
	t.Cleanup(func() {
		changeDefStateBuiltin = previousBuiltin
		changeDefStateShow = previousShow
		changeDefStateReset = previousReset
	})
}
