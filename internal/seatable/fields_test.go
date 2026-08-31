package seatable

import (
	"reflect"
	"testing"
)

func TestFieldValuePrefersMatchedRegion(t *testing.T) {
	row := NeuronRow{Region: "LX, LW", Regions: []string{"LX", "LW"}, MatchedRegions: []string{"LW"}}

	if got := FieldValue(row, "region"); got != "LW" {
		t.Errorf("FieldValue(region) = %q, want the matched region %q", got, "LW")
	}
	if got := FieldValue(NeuronRow{Region: "LX, LW"}, "region"); got != "LX, LW" {
		t.Errorf("FieldValue(region) = %q, want the display join", got)
	}
	if got := FieldValue(row, "not_a_column"); got != "" {
		t.Errorf("FieldValue(not_a_column) = %q, want empty", got)
	}
}

func TestFieldValues(t *testing.T) {
	tests := []struct {
		name  string
		row   NeuronRow
		field string
		want  []string
	}{
		{
			name:  "every region a neuron carries, not just the matched one",
			row:   NeuronRow{Region: "LX, LW", Regions: []string{"LX", "LW"}, MatchedRegions: []string{"LX"}},
			field: "region",
			want:  []string{"LX", "LW"},
		},
		{
			name:  "matched regions when the full set is missing",
			row:   NeuronRow{MatchedRegions: []string{"LX"}},
			field: "region",
			want:  []string{"LX"},
		},
		{
			name:  "a display join splits back into its values",
			row:   NeuronRow{Region: "LX, LW"},
			field: "region",
			want:  []string{"LX", "LW"},
		},
		{
			name:  "no region at all",
			row:   NeuronRow{},
			field: "region",
			want:  nil,
		},
		{
			name:  "single-valued fields yield one value",
			row:   NeuronRow{CellType: "ER"},
			field: "cell_type",
			want:  []string{"ER"},
		},
		{
			name:  "empty fields yield none",
			row:   NeuronRow{},
			field: "cell_type",
			want:  nil,
		},
		{
			name:  "unknown fields yield none",
			row:   NeuronRow{CellType: "ER"},
			field: "not_a_column",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FieldValues(tt.row, tt.field); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FieldValues(%q) = %#v, want %#v", tt.field, got, tt.want)
			}
		})
	}
}
