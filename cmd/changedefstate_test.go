package cmd

import (
	"strings"
	"testing"

	"crantcli/internal/nglstate"
)

func TestRunChangeDefStateRejectsStateWithoutSegmentationLayer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldShow := changeDefStateShow
	oldReset := changeDefStateReset
	changeDefStateShow = false
	changeDefStateReset = false
	t.Cleanup(func() {
		changeDefStateShow = oldShow
		changeDefStateReset = oldReset
	})

	err := runChangeDefState(nil, []string{`{"layers":[{"type":"image","name":"image"}]}`})
	if err == nil {
		t.Fatal("expected invalid state error")
	}
	if !strings.Contains(err.Error(), "no segmentation layer found") {
		t.Fatalf("error = %q, want segmentation-layer validation", err.Error())
	}

	data, readErr := nglstate.ReadDefaultState()
	if readErr != nil {
		t.Fatalf("ReadDefaultState returned error: %v", readErr)
	}
	if len(data) != 0 {
		t.Fatalf("default state was persisted despite validation failure: %s", string(data))
	}
}
