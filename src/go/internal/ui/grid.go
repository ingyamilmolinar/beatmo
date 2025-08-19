package ui

import (
	"image/color"
	"math"
)

const DefaultGridStep = 60 // world-space px between vertices

// LineStyle describes how a grid subdivision should be rendered.
type LineStyle struct {
	Color color.Color
	Width float64
}

// Subdivision defines a set of grid lines drawn between beats.
// Div specifies how many slices a beat is divided into.
// MinPx controls the minimum on-screen spacing (in pixels) required before this
// subdivision becomes visible to avoid crowding.
type Subdivision struct {
	Div   int
	MinPx int
	Style LineStyle
}

// LineGroup contains world-space coordinates for a subdivision's lines.
type LineGroup struct {
	Subdiv Subdivision
	Xs     []float64
	Ys     []float64
}

// Grid encapsulates grid spacing and multiple subdivision levels.
type Grid struct {
	Step float64 // world-space px between beats
	Subs []Subdivision
}

// NewGrid constructs a grid with default subdivision styles.
func NewGrid(step float64) *Grid {
	return &Grid{
		Step: step,
		Subs: []Subdivision{
			{Div: 1, MinPx: 0, Style: LineStyle{Color: colGridLine, Width: 2}},
			{Div: 2, MinPx: 32, Style: LineStyle{Color: colGridHalf, Width: 1.5}},
			{Div: 4, MinPx: 14, Style: LineStyle{Color: colGridQuarter, Width: 1}},
			{Div: 8, MinPx: 10, Style: LineStyle{Color: colGridEighth, Width: 1}},
			{Div: 16, MinPx: 10, Style: LineStyle{Color: colGridSixteenth, Width: 1}},
			{Div: 32, MinPx: 8, Style: LineStyle{Color: colGridThirtySecond, Width: 1}},
		},
	}
}

// MaxDiv returns the finest subdivision factor.
func (g *Grid) MaxDiv() int {
	if len(g.Subs) == 0 {
		return 1
	}
	return g.Subs[len(g.Subs)-1].Div
}

// Unit returns the world-space distance between the smallest subdivisions.
func (g *Grid) Unit() float64 { return g.Step / float64(g.MaxDiv()) }

// UnitPixels returns the on-screen pixel distance between the smallest
// subdivisions for a given camera scale.
func (g *Grid) UnitPixels(scale float64) float64 {
	return float64(g.StepPixels(scale)) / float64(g.MaxDiv())
}

// NodeRadius returns a world-space radius for nodes based on the camera zoom.
// The radius grows with the zoom level but remains a fraction of the smallest
// subdivision so adjacent nodes never overlap. The visual size is capped to a
// reasonable maximum to keep nodes readable when heavily zoomed in.
func (g *Grid) NodeRadius(scale float64) float64 {
	r := 0.4 * g.Unit() // world units, 40% of smallest subdivision
	if screen := r * scale; screen > 16 {
		r = 16 / scale
	}
	return r
}

// SignalRadius returns a world-space radius for travelling pulses. Like
// NodeRadius it scales with zoom and caps the on-screen size to avoid oversized
// pulses at extreme zoom factors.
func (g *Grid) SignalRadius(scale float64) float64 {
	r := 0.2 * g.Unit()
	if screen := r * scale; screen > 6 {
		r = 6 / scale
	}
	return r
}

// Snap world coords to nearest subdivision vertex.
func (g *Grid) Snap(x, y float64) (gx, gy float64, ix, iy int) {
	unit := g.Unit()
	ix = int(math.Round(x / unit))
	iy = int(math.Round(y / unit))
	return float64(ix) * unit, float64(iy) * unit, ix, iy
}

// StepPixels converts a camera scale to an integer pixel spacing between grid
// lines. This helps keep vertical and horizontal gaps consistent across zoom
// levels.
func (g *Grid) StepPixels(scale float64) int {
	px := int(math.Round(scale * g.Step))
	if px < 1 {
		return 1
	}
	return px
}

// Lines returns world-space coordinates for visible grid subdivisions based on
// the camera and screen size.
func (g *Grid) Lines(cam *Camera, screenW, screenH int) []LineGroup {
	stepPx := g.StepPixels(cam.Scale)
	minX, maxX, minY, maxY := visibleWorldRect(cam, screenW, screenH)
	var groups []LineGroup
	if len(g.Subs) == 0 {
		return groups
	}
	maxDiv := g.Subs[len(g.Subs)-1].Div
	drawnX := map[int]struct{}{}
	drawnY := map[int]struct{}{}
	for _, sub := range g.Subs {
		px := stepPx / sub.Div
		if px < sub.MinPx {
			continue
		}
		step := g.Step / float64(sub.Div)
		startI := int(math.Floor(minX / step))
		endI := int(math.Ceil(maxX / step))
		startJ := int(math.Floor(minY / step))
		endJ := int(math.Ceil(maxY / step))
		var xs, ys []float64
		mul := maxDiv / sub.Div
		for i := startI; i <= endI; i++ {
			base := i * mul
			if _, ok := drawnX[base]; ok {
				continue
			}
			xs = append(xs, float64(i)*step)
			drawnX[base] = struct{}{}
		}
		for j := startJ; j <= endJ; j++ {
			base := j * mul
			if _, ok := drawnY[base]; ok {
				continue
			}
			ys = append(ys, float64(j)*step)
			drawnY[base] = struct{}{}
		}
		groups = append(groups, LineGroup{Subdiv: sub, Xs: xs, Ys: ys})
	}
	return groups
}
