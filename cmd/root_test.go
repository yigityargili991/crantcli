package cmd

import (
	"bytes"
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
