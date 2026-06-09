//go:build !headless

package skeletonviewer

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sort"
	"time"

	"crantcli/internal/skeleton"
	"crantcli/internal/skeletonviewer/viewmodel"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type viewerGame struct {
	model      *viewmodel.Model
	prevMouseX int
	prevMouseY int
	mouseReady bool
	shot       bool
}

var errViewerClosed = errors.New("viewer closed")

func Run(sk *skeleton.Skeleton, opts Options) error {
	if err := skeleton.ValidateSkeleton(sk); err != nil {
		return err
	}
	projection := opts.Projection
	if projection == "" {
		projection = skeleton.Projection3D
	}
	game := newViewerGame(sk, projection, opts.InfoLines)
	ebiten.SetWindowSize(1100, 800)
	ebiten.SetWindowTitle(fmt.Sprintf("crantcli skeleton-view %s", sk.RootID))
	if err := ebiten.RunGame(game); err != nil {
		if errors.Is(err, errViewerClosed) {
			return nil
		}
		return err
	}
	return nil
}

func newViewerGame(sk *skeleton.Skeleton, projection skeleton.Projection, infoLines []string) *viewerGame {
	return &viewerGame{model: viewmodel.New(sk, projection, infoLines)}
}

func (g *viewerGame) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		return errViewerClosed
	}

	panStep := 18.0
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		panStep = 45
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		g.model.PanX += panStep
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		g.model.PanX -= panStep
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		g.model.PanY += panStep
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		g.model.PanY -= panStep
	}

	mouseX, mouseY := ebiten.CursorPosition()
	if !g.mouseReady {
		g.prevMouseX = mouseX
		g.prevMouseY = mouseY
		g.mouseReady = true
	}
	dx := float64(mouseX - g.prevMouseX)
	dy := float64(mouseY - g.prevMouseY)
	g.prevMouseX = mouseX
	g.prevMouseY = mouseY
	if g.model.Projection == skeleton.Projection3D && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.model.Yaw += dx * 0.008
		g.model.Pitch += dy * 0.008
		g.model.Pitch = math.Max(-1.45, math.Min(g.model.Pitch, 1.45))
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) || ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		g.model.PanX += dx
		g.model.PanY += dy
	}

	_, wheelY := ebiten.Wheel()
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || wheelY > 0 {
		g.model.Zoom *= 1.15
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || wheelY < 0 {
		g.model.Zoom /= 1.15
	}
	g.model.Zoom = math.Max(0.02, math.Min(g.model.Zoom, 200))

	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyDigit3):
		g.model.SetProjection(skeleton.Projection3D)
	case inpututil.IsKeyJustPressed(ebiten.KeyC):
		g.model.ColorMode = g.model.ColorMode.Next()
	case inpututil.IsKeyJustPressed(ebiten.KeyG):
		g.model.ShowGuides = !g.model.ShowGuides
	case inpututil.IsKeyJustPressed(ebiten.KeyP):
		g.shot = true
	case inpututil.IsKeyJustPressed(ebiten.KeyX):
		g.model.SetProjection(skeleton.ProjectionYZ)
	case inpututil.IsKeyJustPressed(ebiten.KeyY):
		g.model.SetProjection(skeleton.ProjectionXZ)
	case inpututil.IsKeyJustPressed(ebiten.KeyZ):
		g.model.SetProjection(skeleton.ProjectionXY)
	case inpututil.IsKeyJustPressed(ebiten.KeyI):
		g.model.SetProjection(skeleton.ProjectionIso)
	case inpututil.IsKeyJustPressed(ebiten.KeyR):
		g.model.PanX = 0
		g.model.PanY = 0
		g.model.Zoom = 1
		g.model.Yaw = -0.7
		g.model.Pitch = 0.45
	}
	return nil
}

func (g *viewerGame) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 8, G: 10, B: 14, A: 255})
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	points := g.model.ScreenNodes(w, h)
	mouseX, mouseY := ebiten.CursorPosition()
	hovered := g.model.HoveredNode(points, mouseX, mouseY)
	if g.model.Projection == skeleton.Projection3D && g.model.ShowGuides {
		g.drawGuides(screen, g.model.SceneTransform(w, h))
	}

	edges := append([]viewmodel.Edge(nil), g.model.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		a := edges[i]
		b := edges[j]
		depthI := points[a.From].Depth + points[a.To].Depth
		depthJ := points[b.From].Depth + points[b.To].Depth
		return depthI < depthJ
	})

	for _, edge := range edges {
		if edge.From >= len(points) || edge.To >= len(points) {
			continue
		}
		a := points[edge.From]
		b := points[edge.To]
		depth := viewmodel.Clamp01((a.Depth + b.Depth) / 2)
		main := g.model.EdgeColor(edge, depth, 238)
		glow := viewmodel.WithAlpha(main, 54)
		width := g.model.EdgeWidth(edge, depth)
		vector.StrokeLine(screen, float32(a.X), float32(a.Y), float32(b.X), float32(b.Y), width+3.0, glow, true)
		vector.StrokeLine(screen, float32(a.X), float32(a.Y), float32(b.X), float32(b.Y), width, main, true)
	}
	for i, point := range points {
		depth := viewmodel.Clamp01(point.Depth)
		radius := g.model.NodeRadius(i, depth)
		main := g.model.NodeColor(i, depth, 245)
		glow := viewmodel.WithAlpha(main, 54)
		core := viewmodel.Lighten(main, 0.34, 255)
		vector.DrawFilledCircle(screen, float32(point.X), float32(point.Y), radius+2.4, glow, true)
		vector.DrawFilledCircle(screen, float32(point.X), float32(point.Y), radius, main, true)
		vector.DrawFilledCircle(screen, float32(point.X), float32(point.Y), radius*0.45, core, true)
		if i == hovered {
			vector.StrokeCircle(screen, float32(point.X), float32(point.Y), radius+5, 1.5, color.RGBA{R: 255, G: 255, B: 255, A: 230}, true)
		}
	}

	ebitenutil.DebugPrintAt(screen, g.model.OverlayText(), 12, 12)
	if hovered >= 0 {
		ebitenutil.DebugPrintAt(screen, g.model.NodeTooltip(hovered), mouseX+14, mouseY+14)
	}
	if g.shot {
		g.saveScreenshot(screen)
		g.shot = false
	}
}

func (g *viewerGame) drawGuides(screen *ebiten.Image, transform viewmodel.SceneTransform) {
	b := transform.Bounds
	centerX := (b.MinX + b.MaxX) / 2
	centerY := (b.MinY + b.MaxY) / 2
	centerZ := (b.MinZ + b.MaxZ) / 2
	gridColor := color.RGBA{R: 120, G: 132, B: 145, A: 38}
	const steps = 8
	for i := 0; i <= steps; i++ {
		t := float64(i) / steps
		x := viewmodel.Lerp(b.MinX, b.MaxX, t)
		y := viewmodel.Lerp(b.MinY, b.MaxY, t)
		g.drawGuideLine(screen, transform, x, b.MinY, b.MinZ, x, b.MaxY, b.MinZ, gridColor, 1)
		g.drawGuideLine(screen, transform, b.MinX, y, b.MinZ, b.MaxX, y, b.MinZ, gridColor, 1)
	}
	g.drawGuideLine(screen, transform, b.MinX, centerY, centerZ, b.MaxX, centerY, centerZ, color.RGBA{R: 245, G: 88, B: 88, A: 150}, 2)
	g.drawGuideLine(screen, transform, centerX, b.MinY, centerZ, centerX, b.MaxY, centerZ, color.RGBA{R: 92, G: 220, B: 138, A: 150}, 2)
	g.drawGuideLine(screen, transform, centerX, centerY, b.MinZ, centerX, centerY, b.MaxZ, color.RGBA{R: 104, G: 160, B: 250, A: 150}, 2)
	xEnd := transform.Project(0, b.MaxX, centerY, centerZ)
	yEnd := transform.Project(0, centerX, b.MaxY, centerZ)
	zEnd := transform.Project(0, centerX, centerY, b.MaxZ)
	ebitenutil.DebugPrintAt(screen, "X", int(xEnd.X)+4, int(xEnd.Y)+4)
	ebitenutil.DebugPrintAt(screen, "Y", int(yEnd.X)+4, int(yEnd.Y)+4)
	ebitenutil.DebugPrintAt(screen, "Z", int(zEnd.X)+4, int(zEnd.Y)+4)
}

func (g *viewerGame) drawGuideLine(screen *ebiten.Image, transform viewmodel.SceneTransform, x0, y0, z0, x1, y1, z1 float64, clr color.Color, width float32) {
	a := transform.Project(0, x0, y0, z0)
	b := transform.Project(0, x1, y1, z1)
	vector.StrokeLine(screen, float32(a.X), float32(a.Y), float32(b.X), float32(b.Y), width, clr, true)
}

func (g *viewerGame) saveScreenshot(screen *ebiten.Image) {
	path := viewmodel.ScreenshotFileName(g.model.Skeleton.RootID, time.Now())
	if err := writeScreenshot(screen, path); err != nil {
		g.model.Notice = "screenshot failed: " + err.Error()
		return
	}
	g.model.Notice = "screenshot: " + path
}

func writeScreenshot(screen *ebiten.Image, path string) error {
	bounds := screen.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixels := make([]byte, 4*width*height)
	screen.ReadPixels(pixels)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, pixels)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func (g *viewerGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	if outsideWidth < 640 {
		outsideWidth = 640
	}
	if outsideHeight < 480 {
		outsideHeight = 480
	}
	return outsideWidth, outsideHeight
}
