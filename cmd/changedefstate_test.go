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

	// Args normally rejects the bare command first; RunE keeps the guard for an
	// explicit --builtin=false (or --show=false / --reset=false), which Args
	// counts as a mode flag while the parsed value stays false.
	err := runChangeDefState(changeDefStateCmd, nil)
	if err == nil {
		t.Fatal("missing input error = nil")
	}
	for _, want := range []string{"--builtin", "--show", "--reset"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing input error %q lacks %q", err, want)
		}
	}
	if err := runChangeDefState(changeDefStateCmd, []string{"{invalid"}); err == nil {
		t.Fatal("invalid JSON error = nil")
	}
}

func TestChangeDefStateArgsRequiresStateOrMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetCommandFlags(changeDefStateCmd)
	t.Cleanup(func() { resetCommandFlags(changeDefStateCmd) })

	// The no-argument requirement lives in Args so ask's probe can see the mode
	// flags as alternatives to the JSON-state argument.
	err := changeDefStateCmd.Args(changeDefStateCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "provide a JSON state, or use --builtin / --show / --reset") {
		t.Fatalf("bare change-def-state Args error = %v", err)
	}
	if err := changeDefStateCmd.Args(changeDefStateCmd, []string{`{"zoom":4}`}); err != nil {
		t.Fatalf("JSON argument rejected by Args: %v", err)
	}
	if err := changeDefStateCmd.Args(changeDefStateCmd, []string{"/tmp/a.json", "extra"}); err == nil {
		t.Fatal("Args accepted two arguments")
	}
	for _, mode := range changeDefStateModes {
		if err := changeDefStateCmd.Flags().Set(mode, "true"); err != nil {
			t.Fatalf("set --%s: %v", mode, err)
		}
		if err := changeDefStateCmd.Args(changeDefStateCmd, nil); err != nil {
			t.Errorf("--%s with no argument should pass Args: %v", mode, err)
		}
		resetCommandFlags(changeDefStateCmd)
	}

	// At the CLI level the bare command fails without touching any state.
	if _, _, err := runChangeDefStateForTest(t); err == nil || !strings.Contains(err.Error(), "provide a JSON state") {
		t.Errorf("bare change-def-state CLI error = %v", err)
	}
	// An explicit =false mode flag passes Args (it is set) but still fails like
	// before: RunE's guard keeps that path identical to the old behavior.
	if _, _, err := runChangeDefStateForTest(t, "--builtin=false"); err == nil || !strings.Contains(err.Error(), "provide a JSON state") {
		t.Errorf("change-def-state --builtin=false CLI error = %v", err)
	}
	// A real mode flag still satisfies the requirement end to end.
	if out, _, err := runChangeDefStateForTest(t, "--builtin"); err != nil || out == "" {
		t.Errorf("change-def-state --builtin: out = %q, err = %v", out, err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".crantcli", "default_state.json")); !os.IsNotExist(statErr) {
		t.Errorf("failing change-def-state runs touched the default state: %v", statErr)
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

// runChangeDefStateForTest runs the registered change-def-state command with
// clean flags, so one CLI run cannot shape the tests that follow it.
func runChangeDefStateForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	resetCommandFlags(changeDefStateCmd)
	t.Cleanup(func() { resetCommandFlags(changeDefStateCmd) })
	return executeRootForTest(t, append([]string{"change-def-state"}, args...)...)
}

func TestChangeDefStateRejectsInvalidCombinations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	filePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(filePath, []byte(`{"layers":[]}`), 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		// MarkFlagsMutuallyExclusive rejects these before RunE runs.
		{name: "builtin and show", args: []string{"--builtin", "--show"}, wantErr: "none of the others can be"},
		{name: "builtin and reset", args: []string{"--builtin", "--reset"}, wantErr: "none of the others can be"},
		{name: "show and reset", args: []string{"--show", "--reset"}, wantErr: "none of the others can be"},
		// These pair a mode flag with a positional argument, which only RunE rejects.
		{name: "builtin and JSON argument", args: []string{"--builtin", `{"layers":[]}`}, wantErr: "cannot combine --builtin with a JSON or file argument"},
		{name: "builtin and file argument", args: []string{"--builtin", filePath}, wantErr: "cannot combine --builtin with a JSON or file argument"},
		{name: "show and JSON argument", args: []string{"--show", `{"layers":[]}`}, wantErr: "cannot combine --show or --reset with a JSON argument"},
		{name: "reset and JSON argument", args: []string{"--reset", `{"layers":[]}`}, wantErr: "cannot combine --show or --reset with a JSON argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := runChangeDefStateForTest(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("change-def-state %v error = %v, want %q", tt.args, err, tt.wantErr)
			}
			if out != "" {
				t.Fatalf("change-def-state %v wrote %q on error, want no output", tt.args, out)
			}
			if _, statErr := os.Stat(filepath.Join(home, ".crantcli", "default_state.json")); !os.IsNotExist(statErr) {
				t.Fatalf("change-def-state %v touched the default state: %v", tt.args, statErr)
			}
		})
	}
}

func TestRunChangeDefStateWritesStatusToCommandOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetChangeDefStateFlags(t)

	var out bytes.Buffer
	changeDefStateCmd.SetOut(&out)
	t.Cleanup(func() { changeDefStateCmd.SetOut(nil) })

	changeDefStateShow = true
	if err := runChangeDefState(changeDefStateCmd, nil); err != nil {
		t.Fatalf("--show without custom state: %v", err)
	}
	if got, want := out.String(), "No custom default state set (using built-in template)\n"; got != want {
		t.Fatalf("--show output = %q, want %q", got, want)
	}
	out.Reset()

	changeDefStateShow = false
	if err := runChangeDefState(changeDefStateCmd, []string{`{"zoom":4}`}); err != nil {
		t.Fatalf("store state: %v", err)
	}
	if got, want := out.String(), "Default state updated\n"; got != want {
		t.Fatalf("store output = %q, want %q", got, want)
	}
	out.Reset()

	changeDefStateShow = true
	if err := runChangeDefState(changeDefStateCmd, nil); err != nil {
		t.Fatalf("--show with custom state: %v", err)
	}
	if got, want := out.String(), "{\n  \"zoom\": 4\n}\n"; got != want {
		t.Fatalf("--show output = %q, want %q", got, want)
	}
	out.Reset()
	changeDefStateShow = false

	changeDefStateReset = true
	if err := runChangeDefState(changeDefStateCmd, nil); err != nil {
		t.Fatalf("--reset: %v", err)
	}
	if got, want := out.String(), "Reset to built-in default state\n"; got != want {
		t.Fatalf("--reset output = %q, want %q", got, want)
	}
}

func TestGenerateDeprecatedAndHidden(t *testing.T) {
	if !generateCmd.Hidden {
		t.Error("generate must be hidden from --help")
	}
	if generateCmd.Deprecated == "" {
		t.Fatal("generate must be deprecated")
	}
	for _, want := range []string{"change-def-state --builtin", "v0.25.0"} {
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
