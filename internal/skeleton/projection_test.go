package skeleton

import "testing"

func TestParseProjection(t *testing.T) {
	for _, raw := range []string{"3d", "xy", "xz", "yz", "iso"} {
		if _, err := ParseProjection(raw); err != nil {
			t.Fatalf("ParseProjection(%q): %v", raw, err)
		}
	}
	if _, err := ParseProjection("bad"); err == nil {
		t.Fatal("expected error for invalid projection")
	}
}

func TestProjectNodes(t *testing.T) {
	nodes := []SkeletonNode{{ID: 1, X: 10, Y: 20, Z: 30}}
	tests := []struct {
		projection Projection
		wantX      float64
		wantY      float64
		wantDepth  float64
	}{
		{ProjectionXY, 10, 20, 30},
		{ProjectionXZ, 10, 30, 20},
		{ProjectionYZ, 20, 30, 10},
	}
	for _, tt := range tests {
		got := ProjectNodes(nodes, tt.projection)[0]
		if got.X != tt.wantX || got.Y != tt.wantY || got.Depth != tt.wantDepth {
			t.Fatalf("%s projection = %#v, want x=%v y=%v depth=%v", tt.projection, got, tt.wantX, tt.wantY, tt.wantDepth)
		}
	}
}

func TestFitToScreen(t *testing.T) {
	points := []ProjectedNode{
		{ID: 1, X: 0, Y: 0, Depth: 0},
		{ID: 2, X: 10, Y: 10, Depth: 10},
	}
	got := FitToScreen(points, 100, 100, 0, 0, 1, 10)
	if len(got) != 2 {
		t.Fatalf("got %d points, want 2", len(got))
	}
	if got[0].X >= got[1].X || got[0].Y >= got[1].Y {
		t.Fatalf("screen points not ordered as expected: %#v", got)
	}
	if got[0].Depth != 0 || got[1].Depth != 1 {
		t.Fatalf("depths = %v, %v; want 0, 1", got[0].Depth, got[1].Depth)
	}
}
