package skeleton

import "testing"

func TestParseRootID(t *testing.T) {
	_, got, err := ParseRootID(" 720575940610453042 ")
	if err != nil {
		t.Fatalf("ParseRootID returned error: %v", err)
	}
	if got != "720575940610453042" {
		t.Fatalf("root id = %q, want normalized id", got)
	}
}

func TestValidateSkeletonRejectsUnknownEdgeNode(t *testing.T) {
	err := ValidateSkeleton(&Skeleton{
		RootID: "1",
		Nodes:  []SkeletonNode{{ID: 1}},
		Edges:  []SkeletonEdge{{From: 1, To: 2}},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLimitSkeleton(t *testing.T) {
	sk := &Skeleton{
		RootID: "1",
		Nodes: []SkeletonNode{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
		},
		Edges: []SkeletonEdge{
			{From: 1, To: 2},
			{From: 3, To: 5},
			{From: 4, To: 5},
		},
	}

	got := LimitSkeleton(sk, 3)
	if len(got.Nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(got.Nodes))
	}
	if got.Nodes[0].ID != 1 || got.Nodes[1].ID != 3 || got.Nodes[2].ID != 5 {
		t.Fatalf("nodes = %#v, want first/middle/last", got.Nodes)
	}
	if len(got.Edges) != 1 || got.Edges[0] != (SkeletonEdge{From: 3, To: 5}) {
		t.Fatalf("edges = %#v, want only retained endpoints", got.Edges)
	}
}

func TestParseMaxNodes(t *testing.T) {
	if _, err := ParseMaxNodes(-1); err == nil {
		t.Fatal("expected error for negative max nodes")
	}
	if _, err := ParseMaxNodes(1); err == nil {
		t.Fatal("expected error for max nodes of 1")
	}
	if got, err := ParseMaxNodes(2); err != nil || got != 2 {
		t.Fatalf("ParseMaxNodes(2) = %d, %v; want 2, nil", got, err)
	}
}
