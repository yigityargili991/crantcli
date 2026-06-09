package viewmodel

import (
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"strings"
	"time"

	"crantcli/internal/skeleton"
)

type Edge struct {
	From   int
	To     int
	Branch int
}

type ColorMode int

const (
	ColorModeDepth ColorMode = iota
	ColorModeBranch
	ColorModeRadius
	ColorModeL2
)

func (m ColorMode) String() string {
	switch m {
	case ColorModeBranch:
		return "branch"
	case ColorModeRadius:
		return "radius"
	case ColorModeL2:
		return "l2"
	default:
		return "depth"
	}
}

func (m ColorMode) Next() ColorMode {
	if m == ColorModeL2 {
		return ColorModeDepth
	}
	return m + 1
}

type Bounds struct {
	MinX float64
	MaxX float64
	MinY float64
	MaxY float64
	MinZ float64
	MaxZ float64
}

type SceneTransform struct {
	Width       int
	Height      int
	Bounds      Bounds
	centerX     float64
	centerY     float64
	centerZ     float64
	scale       float64
	screenScale float64
	cosY        float64
	sinY        float64
	cosX        float64
	sinX        float64
	panX        float64
	panY        float64
}

type Model struct {
	Skeleton   *skeleton.Skeleton
	Projection skeleton.Projection
	Projected  []skeleton.ProjectedNode
	Edges      []Edge
	Bounds     Bounds
	PanX       float64
	PanY       float64
	Zoom       float64
	Yaw        float64
	Pitch      float64
	InfoLines  []string
	MinRadius  float64
	MaxRadius  float64
	NodeBranch []int
	NodeDegree []int
	BranchN    int
	ColorMode  ColorMode
	ShowGuides bool
	Notice     string
}

func New(sk *skeleton.Skeleton, projection skeleton.Projection, infoLines []string) *Model {
	edges, nodeBranch, nodeDegree, branchN := buildTopology(sk)
	minRadius, maxRadius := radiusBounds(sk.Nodes)
	m := &Model{
		Skeleton:   sk,
		Edges:      edges,
		Bounds:     skeletonBounds(sk.Nodes),
		Zoom:       1,
		Yaw:        -0.7,
		Pitch:      0.45,
		InfoLines:  append([]string(nil), infoLines...),
		MinRadius:  minRadius,
		MaxRadius:  maxRadius,
		NodeBranch: nodeBranch,
		NodeDegree: nodeDegree,
		BranchN:    branchN,
		ShowGuides: true,
	}
	m.SetProjection(projection)
	return m
}

func (m *Model) SetProjection(projection skeleton.Projection) {
	if projection == "" {
		projection = skeleton.Projection3D
	}
	m.Projection = projection
	m.Projected = skeleton.ProjectNodes(m.Skeleton.Nodes, projection)
}

func (m *Model) ScreenNodes(width, height int) []skeleton.ScreenNode {
	if m.Projection != skeleton.Projection3D {
		return skeleton.FitToScreen(m.Projected, width, height, m.PanX, m.PanY, m.Zoom, 36)
	}

	nodes := m.Skeleton.Nodes
	if len(nodes) == 0 {
		return nil
	}
	transform := m.SceneTransform(width, height)
	out := make([]skeleton.ScreenNode, len(nodes))
	for i, node := range nodes {
		out[i] = transform.Project(node.ID, node.X, node.Y, node.Z)
	}
	return out
}

func (m *Model) SceneTransform(width, height int) SceneTransform {
	bounds := m.Bounds
	centerX := (bounds.MinX + bounds.MaxX) / 2
	centerY := (bounds.MinY + bounds.MaxY) / 2
	centerZ := (bounds.MinZ + bounds.MaxZ) / 2
	scale := math.Max(bounds.MaxX-bounds.MinX, math.Max(bounds.MaxY-bounds.MinY, bounds.MaxZ-bounds.MinZ))
	if scale <= 0 {
		scale = 1
	}
	return SceneTransform{
		Width:       width,
		Height:      height,
		Bounds:      bounds,
		centerX:     centerX,
		centerY:     centerY,
		centerZ:     centerZ,
		scale:       scale,
		screenScale: math.Min(float64(width), float64(height)) * 0.42 * m.Zoom,
		cosY:        math.Cos(m.Yaw),
		sinY:        math.Sin(m.Yaw),
		cosX:        math.Cos(m.Pitch),
		sinX:        math.Sin(m.Pitch),
		panX:        m.PanX,
		panY:        m.PanY,
	}
}

func (t SceneTransform) Project(id int64, rawX, rawY, rawZ float64) skeleton.ScreenNode {
	x := (rawX - t.centerX) / t.scale * 2
	y := (rawY - t.centerY) / t.scale * 2
	z := (rawZ - t.centerZ) / t.scale * 2

	xz := x*t.cosY + z*t.sinY
	zz := -x*t.sinY + z*t.cosY
	yy := y*t.cosX - zz*t.sinX
	zz = y*t.sinX + zz*t.cosX

	const camDistance = 3.2
	perspective := camDistance / (camDistance + zz)
	return skeleton.ScreenNode{
		ID:    id,
		X:     xz*t.screenScale*perspective + float64(t.Width)/2 + t.panX,
		Y:     yy*t.screenScale*perspective + float64(t.Height)/2 + t.panY,
		Depth: (zz + 1.8) / 3.6,
	}
}

func (m *Model) OverlayText() string {
	overlay := fmt.Sprintf(
		"root_id: %s\nnodes: %d edges: %d branches: %d\nview: %s zoom: %.2fx color: %s guides: %t\n3D: left-drag rotate, right-drag pan, wheel zoom\nkeys: 3=3D x/y/z/i=2D c=color g=guides p=png r=reset q/Esc=quit",
		m.Skeleton.RootID,
		len(m.Skeleton.Nodes),
		len(m.Skeleton.Edges),
		m.BranchN,
		m.Projection,
		m.Zoom,
		m.ColorMode,
		m.ShowGuides,
	)
	if m.Notice != "" {
		overlay += "\n" + m.Notice
	}
	if len(m.InfoLines) > 0 {
		overlay += "\n\n" + strings.Join(m.InfoLines, "\n")
	}
	return overlay
}

func radiusBounds(nodes []skeleton.SkeletonNode) (float64, float64) {
	minRadius := 0.0
	maxRadius := 0.0
	for _, node := range nodes {
		if node.Radius <= 0 {
			continue
		}
		if minRadius == 0 || node.Radius < minRadius {
			minRadius = node.Radius
		}
		if node.Radius > maxRadius {
			maxRadius = node.Radius
		}
	}
	return minRadius, maxRadius
}

func skeletonBounds(nodes []skeleton.SkeletonNode) Bounds {
	if len(nodes) == 0 {
		return Bounds{}
	}
	bounds := Bounds{
		MinX: nodes[0].X,
		MaxX: nodes[0].X,
		MinY: nodes[0].Y,
		MaxY: nodes[0].Y,
		MinZ: nodes[0].Z,
		MaxZ: nodes[0].Z,
	}
	for _, node := range nodes[1:] {
		bounds.MinX = math.Min(bounds.MinX, node.X)
		bounds.MaxX = math.Max(bounds.MaxX, node.X)
		bounds.MinY = math.Min(bounds.MinY, node.Y)
		bounds.MaxY = math.Max(bounds.MaxY, node.Y)
		bounds.MinZ = math.Min(bounds.MinZ, node.Z)
		bounds.MaxZ = math.Max(bounds.MaxZ, node.Z)
	}
	return bounds
}

type adjacencyEntry struct {
	to   int
	edge int
}

func buildTopology(sk *skeleton.Skeleton) ([]Edge, []int, []int, int) {
	nodeIndex := make(map[int64]int, len(sk.Nodes))
	for i, node := range sk.Nodes {
		nodeIndex[node.ID] = i
	}
	edges := make([]Edge, 0, len(sk.Edges))
	for _, edge := range sk.Edges {
		from, okFrom := nodeIndex[edge.From]
		to, okTo := nodeIndex[edge.To]
		if okFrom && okTo {
			edges = append(edges, Edge{From: from, To: to, Branch: -1})
		}
	}

	adjacency := make([][]adjacencyEntry, len(sk.Nodes))
	for i, edge := range edges {
		adjacency[edge.From] = append(adjacency[edge.From], adjacencyEntry{to: edge.To, edge: i})
		adjacency[edge.To] = append(adjacency[edge.To], adjacencyEntry{to: edge.From, edge: i})
	}
	degree := make([]int, len(sk.Nodes))
	for i := range adjacency {
		degree[i] = len(adjacency[i])
	}

	nodeBranch := make([]int, len(sk.Nodes))
	for i := range nodeBranch {
		nodeBranch[i] = -1
	}
	if len(edges) == 0 {
		for i := range nodeBranch {
			nodeBranch[i] = 0
		}
		return edges, nodeBranch, degree, 1
	}

	starts := make([]int, 0)
	for i, d := range degree {
		if d != 2 && d > 0 {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		starts = append(starts, edges[0].From)
	}

	visited := make([]bool, len(edges))
	branch := 0
	walk := func(start, next, edgeIndex, branchID int) {
		prev := start
		current := next
		currentEdge := edgeIndex
		for {
			if currentEdge < 0 || currentEdge >= len(edges) || visited[currentEdge] {
				return
			}
			visited[currentEdge] = true
			edges[currentEdge].Branch = branchID
			if nodeBranch[prev] < 0 {
				nodeBranch[prev] = branchID
			}
			if nodeBranch[current] < 0 {
				nodeBranch[current] = branchID
			}
			if degree[current] != 2 {
				return
			}
			nextEntry := adjacencyEntry{edge: -1}
			for _, candidate := range adjacency[current] {
				if candidate.to != prev && !visited[candidate.edge] {
					nextEntry = candidate
					break
				}
			}
			if nextEntry.edge < 0 {
				return
			}
			prev, current, currentEdge = current, nextEntry.to, nextEntry.edge
		}
	}

	for _, start := range starts {
		for _, entry := range adjacency[start] {
			if visited[entry.edge] {
				continue
			}
			walk(start, entry.to, entry.edge, branch)
			branch++
		}
	}
	for i, edge := range edges {
		if visited[i] {
			continue
		}
		walk(edge.From, edge.To, i, branch)
		branch++
	}
	if branch == 0 {
		branch = 1
	}
	for i := range nodeBranch {
		if nodeBranch[i] < 0 {
			nodeBranch[i] = 0
		}
	}
	for i := range edges {
		if edges[i].Branch < 0 {
			edges[i].Branch = 0
		}
	}
	return edges, nodeBranch, degree, branch
}

func (m *Model) EdgeWidth(edge Edge, depth float64) float32 {
	zoomScale := math.Sqrt(math.Max(0.7, math.Min(m.Zoom, 5)))
	radius := (m.NodeRadiusNorm(edge.From) + m.NodeRadiusNorm(edge.To)) / 2
	width := (1.15 + depth*1.2 + radius*3.4) * zoomScale
	return float32(math.Max(1.1, math.Min(width, 8.5)))
}

func (m *Model) NodeRadius(index int, depth float64) float32 {
	radius := 2.4 + depth*1.35
	radius += m.NodeRadiusNorm(index) * 2.8
	zoomScale := math.Sqrt(math.Max(0.7, math.Min(m.Zoom, 5)))
	radius *= zoomScale
	return float32(math.Max(2.0, math.Min(radius, 8.5)))
}

func (m *Model) NodeRadiusNorm(index int) float64 {
	if index < 0 || index >= len(m.Skeleton.Nodes) || m.MaxRadius <= m.MinRadius {
		return 0
	}
	nodeRadius := m.Skeleton.Nodes[index].Radius
	if nodeRadius <= 0 {
		return 0
	}
	return Clamp01((nodeRadius - m.MinRadius) / (m.MaxRadius - m.MinRadius))
}

func (m *Model) EdgeColor(edge Edge, depth float64, alpha uint8) color.RGBA {
	switch m.ColorMode {
	case ColorModeBranch:
		return categoricalColor(edge.Branch, alpha)
	case ColorModeRadius:
		return scalarColor((m.NodeRadiusNorm(edge.From)+m.NodeRadiusNorm(edge.To))/2, alpha)
	case ColorModeL2:
		if edge.From >= 0 && edge.From < len(m.Skeleton.Nodes) {
			return hashColor(m.Skeleton.Nodes[edge.From].L2ID, alpha)
		}
	}
	return depthColor(depth, alpha)
}

func (m *Model) NodeColor(index int, depth float64, alpha uint8) color.RGBA {
	switch m.ColorMode {
	case ColorModeBranch:
		if index >= 0 && index < len(m.NodeBranch) {
			return categoricalColor(m.NodeBranch[index], alpha)
		}
	case ColorModeRadius:
		return scalarColor(m.NodeRadiusNorm(index), alpha)
	case ColorModeL2:
		if index >= 0 && index < len(m.Skeleton.Nodes) {
			return hashColor(m.Skeleton.Nodes[index].L2ID, alpha)
		}
	}
	return depthColor(depth, alpha)
}

func (m *Model) HoveredNode(points []skeleton.ScreenNode, mouseX, mouseY int) int {
	if len(points) == 0 {
		return -1
	}
	mx := float64(mouseX)
	my := float64(mouseY)
	best := -1
	bestDistance := math.MaxFloat64
	for i, point := range points {
		radius := float64(m.NodeRadius(i, Clamp01(point.Depth))) + 7
		dx := point.X - mx
		dy := point.Y - my
		distance := dx*dx + dy*dy
		if distance <= radius*radius && distance < bestDistance {
			best = i
			bestDistance = distance
		}
	}
	return best
}

func (m *Model) NodeTooltip(index int) string {
	if index < 0 || index >= len(m.Skeleton.Nodes) {
		return ""
	}
	node := m.Skeleton.Nodes[index]
	branch := 0
	if index < len(m.NodeBranch) {
		branch = m.NodeBranch[index]
	}
	degree := 0
	if index < len(m.NodeDegree) {
		degree = m.NodeDegree[index]
	}
	lines := []string{
		fmt.Sprintf("node: %d", node.ID),
		fmt.Sprintf("xyz: %.0f, %.0f, %.0f", node.X, node.Y, node.Z),
		fmt.Sprintf("radius: %.2f", node.Radius),
		fmt.Sprintf("branch: %d degree: %d", branch, degree),
	}
	if node.L2ID != 0 {
		lines = append(lines, fmt.Sprintf("l2_id: %d", node.L2ID))
	}
	return strings.Join(lines, "\n")
}

func ScreenshotFileName(rootID string, t time.Time) string {
	rootID = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, rootID)
	if rootID == "" {
		rootID = "unknown"
	}
	return filepath.Join(".", fmt.Sprintf("crantcli-skeleton-%s-%s.png", rootID, t.Format("20060102-150405")))
}

func depthColor(depth float64, alpha uint8) color.RGBA {
	depth = Clamp01(depth)
	var r, g, b float64
	if depth < 0.5 {
		t := depth * 2
		r = Lerp(82, 61, t)
		g = Lerp(102, 214, t)
		b = Lerp(214, 198, t)
	} else {
		t := (depth - 0.5) * 2
		r = Lerp(61, 250, t)
		g = Lerp(214, 218, t)
		b = Lerp(198, 106, t)
	}
	return color.RGBA{
		R: uint8(math.Round(r)),
		G: uint8(math.Round(g)),
		B: uint8(math.Round(b)),
		A: alpha,
	}
}

func scalarColor(value float64, alpha uint8) color.RGBA {
	value = Clamp01(value)
	var r, g, b float64
	switch {
	case value < 0.33:
		t := value / 0.33
		r = Lerp(45, 60, t)
		g = Lerp(82, 185, t)
		b = Lerp(190, 165, t)
	case value < 0.66:
		t := (value - 0.33) / 0.33
		r = Lerp(60, 230, t)
		g = Lerp(185, 210, t)
		b = Lerp(165, 90, t)
	default:
		t := (value - 0.66) / 0.34
		r = Lerp(230, 255, t)
		g = Lerp(210, 118, t)
		b = Lerp(90, 74, t)
	}
	return color.RGBA{R: uint8(math.Round(r)), G: uint8(math.Round(g)), B: uint8(math.Round(b)), A: alpha}
}

func categoricalColor(index int, alpha uint8) color.RGBA {
	if index < 0 {
		index = 0
	}
	hue := math.Mod(float64(index)*0.61803398875, 1)
	return hslColor(hue, 0.62, 0.58, alpha)
}

func hashColor(value uint64, alpha uint8) color.RGBA {
	if value == 0 {
		return color.RGBA{R: 150, G: 154, B: 164, A: alpha}
	}
	value ^= value >> 33
	value *= 0xff51afd7ed558ccd
	value ^= value >> 33
	hue := float64(value%360) / 360
	return hslColor(hue, 0.58, 0.58, alpha)
}

func hslColor(h, s, l float64, alpha uint8) color.RGBA {
	h = math.Mod(h, 1)
	if h < 0 {
		h += 1
	}
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		q := l * (1 + s)
		if l >= 0.5 {
			q = l + s - l*s
		}
		p := 2*l - q
		r = hueToRGB(p, q, h+1.0/3.0)
		g = hueToRGB(p, q, h)
		b = hueToRGB(p, q, h-1.0/3.0)
	}
	return color.RGBA{R: uint8(math.Round(r * 255)), G: uint8(math.Round(g * 255)), B: uint8(math.Round(b * 255)), A: alpha}
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
}

func WithAlpha(c color.RGBA, alpha uint8) color.RGBA {
	c.A = alpha
	return c
}

func Lighten(c color.RGBA, amount float64, alpha uint8) color.RGBA {
	amount = Clamp01(amount)
	return color.RGBA{
		R: uint8(math.Round(Lerp(float64(c.R), 255, amount))),
		G: uint8(math.Round(Lerp(float64(c.G), 255, amount))),
		B: uint8(math.Round(Lerp(float64(c.B), 255, amount))),
		A: alpha,
	}
}

func Lerp(a, b, t float64) float64 {
	return a + (b-a)*Clamp01(t)
}

func Clamp01(value float64) float64 {
	return math.Max(0, math.Min(value, 1))
}
