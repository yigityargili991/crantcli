package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseIDs(t *testing.T) {
	got := parseIDs("111, 222\n111\t333")
	want := []string{"111", "222", "333"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIDs() = %#v, want %#v", got, want)
	}
}

// TestStateTransferRejectsBadLabelFields pins the same promise add makes: a
// mistyped field is caught before the command reads the clipboard or queries
// anything.
func TestStateTransferRejectsBadLabelFields(t *testing.T) {
	setFlagForTest(t, stateTransferCmd, "label-by", "cell_typo")

	want := `invalid --label-by "cell_typo"`
	if err := stateTransferCmd.PreRunE(stateTransferCmd, nil); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("PreRunE error = %v, want it to contain %q", err, want)
	}
	if err := stateTransferCmd.RunE(stateTransferCmd, nil); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("RunE error = %v, want it to contain %q", err, want)
	}
}

func TestStateTransferLabelsFlagsMatchAdd(t *testing.T) {
	for _, name := range []string{"labels", "labels-ttl", "labels-hook", "label-by", "label-tags"} {
		stateFlag := stateTransferCmd.Flags().Lookup(name)
		if stateFlag == nil {
			t.Fatalf("state-transfer is missing --%s", name)
		}
		addFlag := addCmd.Flags().Lookup(name)
		if addFlag == nil {
			t.Fatalf("add is missing --%s", name)
		}
		if stateFlag.DefValue != addFlag.DefValue {
			t.Fatalf("state-transfer --%s default = %q, want add default %q", name, stateFlag.DefValue, addFlag.DefValue)
		}
	}
}
