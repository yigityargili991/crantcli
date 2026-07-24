package cmd

import (
	"reflect"
	"testing"
)

func TestParseIDs(t *testing.T) {
	got := parseIDs("111, 222\n111\t333")
	want := []string{"111", "222", "333"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIDs() = %#v, want %#v", got, want)
	}
}

func TestStateTransferLabelsFlagsMatchAdd(t *testing.T) {
	for _, name := range []string{"labels", "labels-ttl", "labels-hook"} {
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
