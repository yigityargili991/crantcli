package skeleton

import (
	"fmt"
	"strconv"
	"strings"
)

// Skeleton is the compact geometry shared by the Python bridge, JSON debug
// output, cache files, and the viewer.
type Skeleton struct {
	RootID string         `json:"root_id"`
	Source string         `json:"source,omitempty"`
	Nodes  []SkeletonNode `json:"nodes"`
	Edges  []SkeletonEdge `json:"edges"`
}

type SkeletonNode struct {
	ID     int64   `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Z      float64 `json:"z"`
	Radius float64 `json:"radius,omitempty"`
	L2ID   uint64  `json:"l2_id,omitempty"`
}

type SkeletonEdge struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

func ParseRootID(raw string) (uint64, string, error) {
	rootID, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid root_id %q: %w", raw, err)
	}
	return rootID, strconv.FormatUint(rootID, 10), nil
}

func ValidateSkeleton(sk *Skeleton) error {
	if sk == nil {
		return fmt.Errorf("bridge returned no skeleton")
	}
	if len(sk.Nodes) == 0 {
		return fmt.Errorf("bridge returned an empty skeleton")
	}
	seen := make(map[int64]struct{}, len(sk.Nodes))
	for _, node := range sk.Nodes {
		if _, ok := seen[node.ID]; ok {
			return fmt.Errorf("bridge returned duplicate node id %d", node.ID)
		}
		seen[node.ID] = struct{}{}
	}
	for _, edge := range sk.Edges {
		if _, ok := seen[edge.From]; !ok {
			return fmt.Errorf("bridge returned edge from unknown node id %d", edge.From)
		}
		if _, ok := seen[edge.To]; !ok {
			return fmt.Errorf("bridge returned edge to unknown node id %d", edge.To)
		}
	}
	return nil
}

func LimitSkeleton(sk *Skeleton, maxNodes int) *Skeleton {
	if sk == nil || maxNodes <= 0 || len(sk.Nodes) <= maxNodes {
		return sk
	}

	limited := &Skeleton{
		RootID: sk.RootID,
		Source: sk.Source,
		Nodes:  make([]SkeletonNode, 0, maxNodes),
		Edges:  []SkeletonEdge{},
	}
	keep := make(map[int64]struct{}, maxNodes)
	step := float64(len(sk.Nodes)-1) / float64(maxNodes-1)
	for i := range maxNodes {
		idx := int(float64(i)*step + 0.5)
		if idx >= len(sk.Nodes) {
			idx = len(sk.Nodes) - 1
		}
		node := sk.Nodes[idx]
		if _, ok := keep[node.ID]; ok {
			continue
		}
		keep[node.ID] = struct{}{}
		limited.Nodes = append(limited.Nodes, node)
	}
	for _, edge := range sk.Edges {
		if _, ok := keep[edge.From]; !ok {
			continue
		}
		if _, ok := keep[edge.To]; !ok {
			continue
		}
		limited.Edges = append(limited.Edges, edge)
	}
	return limited
}
