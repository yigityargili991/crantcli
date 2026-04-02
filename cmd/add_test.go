package cmd

import (
	"testing"

	"crantinject/internal/seatable"
)

func TestResolveAddRegionFilter(t *testing.T) {
	t.Run("bundle aliases region", func(t *testing.T) {
		got, err := resolveAddRegionFilter("", "LX")
		if err != nil {
			t.Fatalf("resolveAddRegionFilter returned error: %v", err)
		}
		if got != "LX" {
			t.Fatalf("resolveAddRegionFilter = %q, want %q", got, "LX")
		}
	})

	t.Run("conflicting flags", func(t *testing.T) {
		_, err := resolveAddRegionFilter("CX", "LX")
		if err == nil {
			t.Fatal("expected conflict error when both region and bundle are set")
		}
	})
}

func TestValidateAddInputs(t *testing.T) {
	err := validateAddInputs(&seatable.Filters{Region: "LX"}, false, false, "", false)
	if err != nil {
		t.Fatalf("validateAddInputs returned error: %v", err)
	}

	err = validateAddInputs(&seatable.Filters{}, false, false, "", false)
	if err == nil {
		t.Fatal("expected missing-filters error")
	}
}
