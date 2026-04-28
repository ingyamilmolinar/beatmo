package ui

import (
	"image/color"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

const DefaultGridStep = 60 // world-space px between vertices

// LineStyle describes how a grid subdivision should be rendered.
// Width specifies the desired on-screen thickness in pixels.
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

	// cache for grid lines: reused when camera and screen are unchanged
	cacheValid     bool
	cacheScale     float64
	cacheOffX      float64
	cacheOffY      float64
	cacheW, cacheH int
	cacheGroups    []LineGroup
	cacheStep      float64
	cacheSubSig    uint64

	// precomputed signature of Subs (Div, MinPx); updated by SetSubs/NewGrid
	subSig uint64
}

// NewGrid constructs a grid with default subdivision styles.
func NewGrid(step float64) *Grid {
	g := &Grid{
		Step: step,
		Subs: []Subdivision{
			{Div: 1, MinPx: 0, Style: LineStyle{Color: colGridLine, Width: 1}},
			{Div: 2, MinPx: 32, Style: LineStyle{Color: colGridHalf, Width: 1}},
			{Div: 4, MinPx: 14, Style: LineStyle{Color: colGridQuarter, Width: 1}},
			{Div: 8, MinPx: 10, Style: LineStyle{Color: colGridEighth, Width: 1}},
			{Div: 16, MinPx: 10, Style: LineStyle{Color: colGridSixteenth, Width: 1}},
			{Div: 32, MinPx: 8, Style: LineStyle{Color: colGridThirtySecond, Width: 1}},
		},
	}
	g.recomputeSubSig()
	return g
}

// SetSubs replaces the grid subdivision configuration and recomputes internal
// caches. Prefer using this instead of assigning Subs directly so caches stay
// coherent.
func (g *Grid) SetSubs(subs []Subdivision) {
	g.Subs = subs
	g.recomputeSubSig()
	g.cacheValid = false
}

func (g *Grid) recomputeSubSig() {
	var sig uint64
	for i := range g.Subs {
		sig = sig*131 + uint64(g.Subs[i].Div*257+g.Subs[i].MinPx)
	}
	g.subSig = sig
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
	// Base world radius capped so two adjacent nodes at the finest
	// subdivision never overlap (<= 0.8*Unit apart).
	base := 0.4 * g.Unit()
	// Apply min/max on-screen sizes for readability while zoomed in/out.
	// Mobile uses larger minimums for tap targets and visibility.
	p := Profile()
	minPx := float64(p.NodeMinPx)
	maxPx := float64(p.NodeMaxPx)
	r := base
	scr := r * scale
	if scr < minPx {
		// Request at least minPx on screen when possible.
		r = minPx / scale
	}
	if scr > maxPx {
		r = maxPx / scale
	}
	// Never exceed the base (non-overlap) bound.
	if r > base {
		r = base
	}
	return r
}

// SignalRadius returns a world-space radius for travelling pulses. Like
// NodeRadius it scales with zoom and caps the on-screen size to avoid oversized
// pulses at extreme zoom factors.
func (g *Grid) SignalRadius(scale float64) float64 {
	// Base follows grid unit for proportional look.
	base := 0.2 * g.Unit()
	// Ensure pulses remain visible at bird's‑eye zoom levels.
	const minPx = 4.0
	const maxPx = 6.0
	r := base
	scr := r * scale
	if scr < minPx {
		r = minPx / scale
		scr = minPx
	}
	if scr > maxPx {
		r = maxPx / scale
	}
	return r
}

// EdgeThickness returns a world-space thickness for connection lines so they
// remain one screen pixel wide regardless of zoom level.
func (g *Grid) EdgeThickness(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	return float64(Profile().EdgeThickMul) / scale
}

// EdgeArrowSize returns the world-space length of arrow heads. Keeping them at
// one subdivision unit ensures connection arrows remain understated.
func (g *Grid) EdgeArrowSize() float64 {
	// Keep arrowheads at a fixed fraction of a beat in world-space so they
	// remain visually consistent across subdivision changes. Using Step (px
	// per beat in world units) decouples size from MaxDiv. The fraction is
	// sourced from DESIGN.md geometry.edge-arrow-step-fraction.
	return genGeomEdgeArrowStepFraction * g.Step
}

// Snap world coords to nearest subdivision vertex.
func (g *Grid) Snap(x, y float64) (gx, gy float64, ix, iy int) {
	unit := g.Unit()
	ix = int(math.Round(x / unit))
	iy = int(math.Round(y / unit))
	return float64(ix) * unit, float64(iy) * unit, ix, iy
}

// BeatSubdivision splits a subdivision index into a beat count and a reduced
// fraction (num/den) representing the remaining sub-beat portion.
func (g *Grid) BeatSubdivision(idx int) (beat, num, den int) {
	div := g.MaxDiv()
	if div <= 0 {
		return 0, 0, 1
	}
	beat = int(math.Floor(float64(idx) / float64(div)))
	rem := idx - beat*div
	if rem == 0 {
		return beat, 0, 1
	}
	d := utils.GCD(rem, div)
	return beat, rem / d, div / d
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
	// Use precomputed subdivision signature; recomputed via SetSubs/NewGrid.
	subSig := g.subSig
	// Fast path: if camera, screen, step and subdivision signature are unchanged,
	// reuse cached lines. Camera offsets are snapped to integer pixels by Camera.Snap().
	if g.cacheValid &&
		g.cacheScale == cam.Scale &&
		g.cacheOffX == cam.OffsetX &&
		g.cacheOffY == cam.OffsetY &&
		g.cacheW == screenW && g.cacheH == screenH &&
		g.cacheStep == g.Step && g.cacheSubSig == subSig {
		return g.cacheGroups
	}
	stepPx := g.StepPixels(cam.Scale)
	minX, maxX, minY, maxY := visibleWorldRect(cam, screenW, screenH)
	// pad the visible rectangle by one beat on each side so grid lines extend
	// beyond the screen edges. This avoids gaps when panning and gives the
	// impression of an infinite lattice.
	minX -= g.Step
	maxX += g.Step
	minY -= g.Step
	maxY += g.Step
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
	// Update cache
	g.cacheValid = true
	g.cacheScale = cam.Scale
	g.cacheOffX = cam.OffsetX
	g.cacheOffY = cam.OffsetY
	g.cacheW, g.cacheH = screenW, screenH
	g.cacheGroups = groups
	g.cacheStep = g.Step
	g.cacheSubSig = subSig
	return groups
}
