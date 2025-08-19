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
			{Div: 4, MinPx: 14, Style: LineStyle{Color: colGridQuarter, Width: 1}},
			{Div: 2, MinPx: 32, Style: LineStyle{Color: colGridHalf, Width: 1.5}},
			{Div: 8, MinPx: 10, Style: LineStyle{Color: colGridEighth, Width: 1}},
			{Div: 16, MinPx: 10, Style: LineStyle{Color: colGridSixteenth, Width: 1}},
			{Div: 32, MinPx: 8, Style: LineStyle{Color: colGridThirtySecond, Width: 1}},
		},
	}
}

// Snap world coords to nearest vertex.
func (g *Grid) Snap(x, y float64) (gx, gy float64, ix, iy int) {
	ix = int(math.Round(x / g.Step))
	iy = int(math.Round(y / g.Step))
	return float64(ix) * g.Step, float64(iy) * g.Step, ix, iy
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
		for i := startI; i <= endI; i++ {
			xs = append(xs, float64(i)*step)
		}
		for j := startJ; j <= endJ; j++ {
			ys = append(ys, float64(j)*step)
		}
		groups = append(groups, LineGroup{Subdiv: sub, Xs: xs, Ys: ys})
	}
	return groups
}
