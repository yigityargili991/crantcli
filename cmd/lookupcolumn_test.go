package cmd

import (
	"strings"
	"testing"

	"crantcli/internal/seatable"
)

func TestFormatLookupColumnOutput(t *testing.T) {
	neuron := &seatable.NeuronPositionRow{
		RootID: "720575940610453042",
		Region: "CX, LW",
	}

	got := formatLookupColumnOutput(neuron)
	want := "CX, LW\t720575940610453042"
	if got != want {
		t.Fatalf("formatLookupColumnOutput() = %q, want %q", got, want)
	}
}

func TestLookupColumnDeprecatedAndHidden(t *testing.T) {
	if !lookupColumnCmd.Hidden {
		t.Error("lookup-column must be hidden from --help")
	}
	if lookupColumnCmd.Deprecated == "" {
		t.Fatal("lookup-column must be deprecated")
	}
	for _, want := range []string{"root-info", "v0.24.0"} {
		if !strings.Contains(lookupColumnCmd.Deprecated, want) {
			t.Errorf("lookup-column deprecation warning %q lacks %q", lookupColumnCmd.Deprecated, want)
		}
	}
}
