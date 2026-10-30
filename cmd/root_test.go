// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeRootForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		rootCmd.SilenceUsage = false
	})

	err := rootCmd.Execute()
	return out.String(), errOut.String(), err
}

// SEC-003 regression: a read-only query command with no token must fail with
// guidance instead of prompting and silently writing credentials.
func TestRequiresTokenErrorsWithoutToken(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("CRANTTABLE_TOKEN", "")
	t.Setenv("CRANTTABLE_TOKEN_FILE", "")
	originalGetAPIToken := getAPIToken
	getAPIToken = func() string { return "" }
	t.Cleanup(func() { getAPIToken = originalGetAPIToken })

	output, errOutput, err := executeRootForTest(t, "list", "cell_type")
	if err == nil {
		t.Fatal("expected missing-token error")
	}
	if !strings.Contains(err.Error(), "crantcli setup") {
		t.Fatalf("error = %q, want guidance to run setup", err.Error())
	}
	if strings.Contains(output, "Usage:") || strings.Contains(errOutput, "Usage:") {
		t.Fatalf("runtime error printed command usage: stdout=%q stderr=%q", output, errOutput)
	}

	entries, readErr := os.ReadDir(tempHome)
	if readErr != nil {
		t.Fatalf("read temp home: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("query command wrote files without a token: %v", entries)
	}
}

// The annotation-gated path (root.go PersistentPreRunE) and the dynamic one
// (add's RunE, which decides from its flags) must reject a tokenless run with
// the same message, so users see one guidance string whatever the command.
func TestMissingSeaTableTokenMessageIsShared(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CRANTTABLE_TOKEN", "")
	t.Setenv("CRANTTABLE_TOKEN_FILE", "")
	originalGetAPIToken := getAPIToken
	getAPIToken = func() string { return "" }
	t.Cleanup(func() { getAPIToken = originalGetAPIToken })

	_, _, annotatedErr := executeRootForTest(t, "list", "cell_type")
	if annotatedErr == nil {
		t.Fatal("list without a token should fail")
	}
	if !errors.Is(annotatedErr, errNoSeaTableToken) {
		t.Fatalf("annotation-gated error = %v, want the canonical missing-token error", annotatedErr)
	}

	isolateAddCommandRun(t)
	getAPIToken = func() string { return "" }
	dynamicErr := addCmd.RunE(addCmd, nil)
	if !errors.Is(dynamicErr, errNoSeaTableToken) {
		t.Fatalf("add query error = %v, want the canonical missing-token error", dynamicErr)
	}

	if annotatedErr.Error() != dynamicErr.Error() {
		t.Fatalf("token errors differ:\n annotation path: %q\n dynamic path:    %q", annotatedErr, dynamicErr)
	}
	if want := "no SeaTable token configured; run 'crantcli setup' to store one, or set CRANTTABLE_TOKEN / CRANTTABLE_TOKEN_FILE"; annotatedErr.Error() != want {
		t.Fatalf("token error = %q, want %q", annotatedErr, want)
	}
}

// The setup command itself remains the explicit credential-writing path.
func TestSetupRequiresTerminal(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	_, _, err := executeRootForTest(t, "setup")
	if err == nil {
		t.Fatal("expected non-terminal error from setup")
	}
	if !strings.Contains(err.Error(), "not a terminal") {
		t.Fatalf("error = %q, want non-terminal message", err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(tempHome, ".crantcli")); !os.IsNotExist(statErr) {
		t.Fatal("setup wrote credentials despite non-terminal stdin")
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		err  error
		want int
	}{
		{"success", checkCaveCmd, nil, 0},
		{"stale root IDs", checkCaveCmd, errStaleFound, 1},
		{"check-cave failure", checkCaveCmd, errors.New("boom"), 2},
		{"other command failure", addCmd, errors.New("boom"), 1},
	}
	for _, tt := range tests {
		if got := exitCode(tt.cmd, tt.err); got != tt.want {
			t.Errorf("%s: exitCode = %d, want %d", tt.name, got, tt.want)
		}
	}
}
