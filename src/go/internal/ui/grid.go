package ui

import (
	"fmt"
	"image/color"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

const DefaultGridStep = 60 // world-space px between vertices

// Width is in screen pixels.
type LineStyle struct {
	Color color.Color
	Width float64
}

// Div slices a beat; MinPx is the spacing below which the subdivision hides.
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

// The ONE subdivision-to-token table, so a theme change re-resolves every line from the live tokens.
func gridLineColorForDiv(div int) color.RGBA {
	switch div {
	case 1:
		return colGridLine
	case 2:
		return colGridHalf
	case 4:
		return colGridQuarter
	case 8:
		return colGridEighth
	case 16:
		return colGridSixteenth
	case 32:
		return colGridThirtySecond
	default:
		return colGridLine
	}
}

func (g *Grid) RefreshLineColors() {
	if g == nil {
		return
	}
	for i := range g.Subs {
		g.Subs[i].Style.Color = gridLineColorForDiv(g.Subs[i].Div)
	}
}

// NewGrid constructs a grid with default subdivision styles.
func NewGrid(step float64) *Grid {
	g := &Grid{
		Step: step,
		Subs: []Subdivision{
			{Div: 1, MinPx: 0, Style: LineStyle{Color: gridLineColorForDiv(1), Width: 1}},
			{Div: 2, MinPx: 32, Style: LineStyle{Color: gridLineColorForDiv(2), Width: 1}},
			{Div: 4, MinPx: 14, Style: LineStyle{Color: gridLineColorForDiv(4), Width: 1}},
			{Div: 8, MinPx: 10, Style: LineStyle{Color: gridLineColorForDiv(8), Width: 1}},
			{Div: 16, MinPx: 10, Style: LineStyle{Color: gridLineColorForDiv(16), Width: 1}},
			{Div: 32, MinPx: 8, Style: LineStyle{Color: gridLineColorForDiv(32), Width: 1}},
		},
	}
	g.recomputeSubSig()
	return g
}

// Use this rather than assigning Subs, so the caches stay coherent.
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

func (g *Grid) UnitPixels(scale float64) float64 {
	return float64(g.StepPixels(scale)) / float64(g.MaxDiv())
}

// A fraction of the finest subdivision so adjacent nodes never overlap, capped on screen.
func (g *Grid) NodeRadius(scale float64) float64 {
	base := 0.4 * g.Unit()
	// Mobile has larger minimums for tap targets.
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

// One screen pixel at any zoom.
func (g *Grid) EdgeThickness(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	return float64(Profile().EdgeThickMul) / scale
}

func (g *Grid) EdgeArrowSize() float64 {
	// A fraction of a beat (Step), so the size is independent of MaxDiv.
	return genGeomEdgeArrowStepFraction * g.Step
}

// Snap world coords to nearest subdivision vertex.
func (g *Grid) Snap(x, y float64) (gx, gy float64, ix, iy int) {
	unit := g.Unit()
	ix = int(math.Round(x / unit))
	iy = int(math.Round(y / unit))
	return float64(ix) * unit, float64(iy) * unit, ix, iy
}

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

// The sign belongs to the whole coordinate: 2.5 beats left reads "-2:1/2", not floor's "-3:1/2";
// an exact beat drops the fraction.
func (g *Grid) BeatAxisLabel(idx int) string {
	sign := ""
	if idx < 0 {
		sign = "-"
		idx = -idx
	}
	beat, num, den := g.BeatSubdivision(idx)
	if num == 0 {
		return fmt.Sprintf("%s%d", sign, beat)
	}
	return fmt.Sprintf("%s%d:%d/%d", sign, beat, num, den)
}

// 1-based beat plus the raw fraction num/den with den = MaxDiv: idx 4 at div 8 is "Beat 1, 4/8".
// Negative idx and div <= 0 clamp to beat 1.
func (g *Grid) BeatSlot(idx int) (beat, num, den int) {
	den = g.MaxDiv()
	if den <= 0 {
		return 1, 0, 1
	}
	if idx < 0 {
		idx = 0
	}
	beat = idx/den + 1
	num = idx % den
	return beat, num, den
}

// Integer spacing keeps horizontal and vertical gaps equal across zoom levels.
func (g *Grid) StepPixels(scale float64) int {
	px := int(math.Round(scale * g.Step))
	if px < 1 {
		return 1
	}
	return px
}

func (g *Grid) Lines(cam *Camera, screenW, screenH int) []LineGroup {
	// Use precomputed subdivision signature; recomputed via SetSubs/NewGrid.
	subSig := g.subSig
	// Camera offsets are snapped to whole pixels, so an unchanged signature can reuse the lines.
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
	// One beat of padding each side, so panning never shows a gap at the edges.
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
