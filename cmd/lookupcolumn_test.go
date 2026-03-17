package cmd

import (
	"math"
	"testing"
)

func TestParsePos(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantX   float64
		wantY   float64
		wantZ   float64
		wantErr bool
	}{
		{"valid integers", "100,200,300", 100, 200, 300, false},
		{"valid floats", "100.5,200.3,300.1", 100.5, 200.3, 300.1, false},
		{"with spaces", " 100.5 , 200.3 , 300.1 ", 100.5, 200.3, 300.1, false},
		{"two values", "100,200", 0, 0, 0, true},
		{"four values", "100,200,300,400", 0, 0, 0, true},
		{"empty string", "", 0, 0, 0, true},
		{"non-numeric x", "abc,200,300", 0, 0, 0, true},
		{"non-numeric y", "100,abc,300", 0, 0, 0, true},
		{"non-numeric z", "100,200,abc", 0, 0, 0, true},
		{"negative values", "-100.5,-200.3,-300.1", -100.5, -200.3, -300.1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, z, err := parsePos(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePos(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr {
				if x != tt.wantX || y != tt.wantY || z != tt.wantZ {
					t.Fatalf("parsePos(%q) = (%v, %v, %v), want (%v, %v, %v)",
						tt.input, x, y, z, tt.wantX, tt.wantY, tt.wantZ)
				}
			}
		})
	}
}

func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name string
		x1, y1, z1 float64
		x2, y2, z2 float64
		want       float64
	}{
		{"same point", 0, 0, 0, 0, 0, 0, 0},
		{"unit x", 0, 0, 0, 1, 0, 0, 1},
		{"unit y", 0, 0, 0, 0, 1, 0, 1},
		{"unit z", 0, 0, 0, 0, 0, 1, 1},
		{"3-4-5 triangle", 0, 0, 0, 3, 4, 0, 5},
		{"all axes", 1, 2, 3, 4, 6, 3, 5},
		{"negative coords", -1, -2, -3, 2, 2, -3, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := euclideanDistance(tt.x1, tt.y1, tt.z1, tt.x2, tt.y2, tt.z2)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("euclideanDistance(%v,%v,%v, %v,%v,%v) = %v, want %v",
					tt.x1, tt.y1, tt.z1, tt.x2, tt.y2, tt.z2, got, tt.want)
			}
		})
	}
}
