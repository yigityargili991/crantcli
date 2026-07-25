package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	_, _, err := executeRootForTest(t, "list", "cell_type")
	if err == nil {
		t.Fatal("expected missing-token error")
	}
	if !strings.Contains(err.Error(), "crantcli setup") {
		t.Fatalf("error = %q, want guidance to run setup", err.Error())
	}

	entries, readErr := os.ReadDir(tempHome)
	if readErr != nil {
		t.Fatalf("read temp home: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("query command wrote files without a token: %v", entries)
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
