package viewmodel

import (
	"math"
	"strings"
	"testing"
	"time"

	"crantcli/internal/skeleton"
)

func testViewerSkeleton() *skeleton.Skeleton {
	return &skeleton.Skeleton{
		RootID: "111",
		Nodes: []skeleton.SkeletonNode{
			{ID: 1, X: 0, Y: 0, Z: 0},
			{ID: 2, X: 10, Y: 0, Z: 0},
			{ID: 3, X: 10, Y: 5, Z: 3},
		},
		Edges: []skeleton.SkeletonEdge{
			{From: 1, To: 2},
			{From: 2, To: 3},
		},
	}
}

func TestViewerDefaultsTo3D(t *testing.T) {
	m := New(testViewerSkeleton(), "", nil)
	if m.Projection != skeleton.Projection3D {
		t.Fatalf("projection = %q, want 3d", m.Projection)
	}
}

func TestViewerScreenNodes3DNormalizesWithoutWindow(t *testing.T) {
	m := New(testViewerSkeleton(), skeleton.Projection3D, nil)
	points := m.ScreenNodes(800, 600)
	if len(points) != 3 {
		t.Fatalf("got %d points, want 3", len(points))
	}
	for _, point := range points {
		if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsNaN(point.Depth) {
			t.Fatalf("screen point contains NaN: %#v", point)
		}
		if point.X < 0 || point.X > 800 || point.Y < 0 || point.Y > 600 {
			t.Fatalf("screen point outside viewport: %#v", point)
		}
		if point.Depth < 0 || point.Depth > 1 {
			t.Fatalf("depth = %v, want normalized 0..1", point.Depth)
		}
	}
}

func TestViewerOverlayIncludesRootInfo(t *testing.T) {
	m := New(testViewerSkeleton(), skeleton.Projection3D, []string{"root_info", "cell: EPG/PEG", "cave: ok"})
	overlay := m.OverlayText()
	for _, want := range []string{"root_id: 111", "root_info", "cell: EPG/PEG", "cave: ok"} {
		if !strings.Contains(overlay, want) {
			t.Fatalf("overlay missing %q:\n%s", want, overlay)
		}
	}
}

func TestViewerNodeRadiusUsesSkeletonRadius(t *testing.T) {
	sk := testViewerSkeleton()
	sk.Nodes[0].Radius = 10
	sk.Nodes[1].Radius = 100
	m := New(sk, skeleton.Projection3D, nil)

	small := m.NodeRadius(0, 0.5)
	large := m.NodeRadius(1, 0.5)
	if large <= small {
		t.Fatalf("large radius = %v, small radius = %v; want large > small", large, small)
	}
}

func TestViewerTopologyFindsBranches(t *testing.T) {
	sk := &skeleton.Skeleton{
		RootID: "111",
		Nodes: []skeleton.SkeletonNode{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4},
		},
		Edges: []skeleton.SkeletonEdge{
			{From: 1, To: 2},
			{From: 2, To: 3},
			{From: 2, To: 4},
		},
	}
	m := New(sk, skeleton.Projection3D, nil)
	if m.BranchN != 3 {
		t.Fatalf("branchN = %d, want 3", m.BranchN)
	}
	for _, edge := range m.Edges {
		if edge.Branch < 0 {
			t.Fatalf("edge has unset branch: %#v", edge)
		}
	}
}

func TestViewerColorModes(t *testing.T) {
	if ColorModeDepth.Next() != ColorModeBranch {
		t.Fatal("depth should cycle to branch")
	}
	if ColorModeL2.Next() != ColorModeDepth {
		t.Fatal("l2 should cycle to depth")
	}
	if ColorModeRadius.String() != "radius" {
		t.Fatalf("radius mode string = %q", ColorModeRadius)
	}
}

func TestScreenshotFileName(t *testing.T) {
	got := ScreenshotFileName("root/id", time.Date(2026, 5, 16, 10, 11, 12, 0, time.UTC))
	if !strings.Contains(got, "crantcli-skeleton-root-id-20260516-101112.png") {
		t.Fatalf("screenshot file name = %q", got)
	}
}
