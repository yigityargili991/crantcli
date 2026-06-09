package skeleton

import (
	"fmt"
	"math"
)

type Projection string

const (
	Projection3D  Projection = "3d"
	ProjectionXY  Projection = "xy"
	ProjectionXZ  Projection = "xz"
	ProjectionYZ  Projection = "yz"
	ProjectionIso Projection = "iso"
)

type ProjectedNode struct {
	ID    int64
	X     float64
	Y     float64
	Depth float64
}

type Bounds struct {
	MinX     float64
	MaxX     float64
	MinY     float64
	MaxY     float64
	MinDepth float64
	MaxDepth float64
}

type ScreenNode struct {
	ID    int64
	X     float64
	Y     float64
	Depth float64
}

func ParseProjection(raw string) (Projection, error) {
	switch Projection(raw) {
	case Projection3D, ProjectionXY, ProjectionXZ, ProjectionYZ, ProjectionIso:
		return Projection(raw), nil
	default:
		return "", fmt.Errorf("invalid projection %q; expected 3d, xy, xz, yz, or iso", raw)
	}
}

func ProjectNodes(nodes []SkeletonNode, projection Projection) []ProjectedNode {
	out := make([]ProjectedNode, len(nodes))
	for i, node := range nodes {
		switch projection {
		case ProjectionXZ:
			out[i] = ProjectedNode{ID: node.ID, X: node.X, Y: node.Z, Depth: node.Y}
		case ProjectionYZ:
			out[i] = ProjectedNode{ID: node.ID, X: node.Y, Y: node.Z, Depth: node.X}
		case ProjectionIso:
			x := (node.X - node.Y) * math.Cos(math.Pi/6)
			y := node.Z + (node.X+node.Y)*math.Sin(math.Pi/6)
			out[i] = ProjectedNode{ID: node.ID, X: x, Y: y, Depth: node.X + node.Y + node.Z}
		default:
			out[i] = ProjectedNode{ID: node.ID, X: node.X, Y: node.Y, Depth: node.Z}
		}
	}
	return out
}

func ProjectedBounds(nodes []ProjectedNode) Bounds {
	if len(nodes) == 0 {
		return Bounds{}
	}
	b := Bounds{
		MinX: nodes[0].X, MaxX: nodes[0].X,
		MinY: nodes[0].Y, MaxY: nodes[0].Y,
		MinDepth: nodes[0].Depth, MaxDepth: nodes[0].Depth,
	}
	for _, node := range nodes[1:] {
		b.MinX = math.Min(b.MinX, node.X)
		b.MaxX = math.Max(b.MaxX, node.X)
		b.MinY = math.Min(b.MinY, node.Y)
		b.MaxY = math.Max(b.MaxY, node.Y)
		b.MinDepth = math.Min(b.MinDepth, node.Depth)
		b.MaxDepth = math.Max(b.MaxDepth, node.Depth)
	}
	return b
}

func FitToScreen(nodes []ProjectedNode, width, height int, panX, panY, zoom float64, padding float64) []ScreenNode {
	if len(nodes) == 0 || width <= 0 || height <= 0 {
		return nil
	}
	b := ProjectedBounds(nodes)
	spanX := math.Max(b.MaxX-b.MinX, 1)
	spanY := math.Max(b.MaxY-b.MinY, 1)
	usableW := math.Max(float64(width)-2*padding, 1)
	usableH := math.Max(float64(height)-2*padding, 1)
	scale := math.Min(usableW/spanX, usableH/spanY) * zoom
	centerX := (b.MinX + b.MaxX) / 2
	centerY := (b.MinY + b.MaxY) / 2
	screenCenterX := float64(width)/2 + panX
	screenCenterY := float64(height)/2 + panY

	out := make([]ScreenNode, len(nodes))
	for i, node := range nodes {
		out[i] = ScreenNode{
			ID:    node.ID,
			X:     (node.X-centerX)*scale + screenCenterX,
			Y:     (node.Y-centerY)*scale + screenCenterY,
			Depth: normalizedDepth(node.Depth, b),
		}
	}
	return out
}

func normalizedDepth(depth float64, b Bounds) float64 {
	span := b.MaxDepth - b.MinDepth
	if span <= 0 {
		return 0.5
	}
	return (depth - b.MinDepth) / span
}
