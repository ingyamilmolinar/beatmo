package ui

// macroCardDialRects is the ONE place a dial's rect is computed, for the hit areas (screen rect) and the draw
// (sprite rect), so the two cannot disagree.

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// Comparable, so a card's sprite cache key can hold the array.
type macroDialSlot struct {
	id      string
	label   string
	reading int // 0…100, a pure function of the instrument's values
	dim     bool
	focused bool // the Synth tab's focused dial (synth_focus.go)
}

// Tighter on a compact card so two rows of knobs fit the 720p panel.
func macroCardInset() int {
	if synthCompactCards() {
		return SpaceXS
	}
	return SpaceXS + 2
}

func macroCardTopH() int {
	if synthCompactCards() {
		return StyledTextHeight(RoleBody)
	}
	return StyledTextHeight(RoleBody) + 2 + StyledTextHeight(RoleCaption)
}

// Production cards are title + dials only: the explanation moves to the graph caption, where it is never
// truncated, and there is no drill to point at.
func synthCompactCards() bool { return !devToolsOn() }

func macroDialRowH() int {
	if synthCompactCards() {
		// Knob style: the arc with the reading inside, the label under it.
		return Profile().DensityValues().SynthKnobD + 1 + StyledTextHeight(RoleMicro)
	}
	h := Profile().DensityValues().SynthDialD
	if t := 2 * StyledTextHeight(RoleMicro); t > h {
		h = t
	}
	return h
}

func macroDialCellMinW() int {
	if synthCompactCards() {
		return max(Profile().DensityValues().SynthKnobD+SpaceSM, compactDialLabelW)
	}
	return Profile().DensityValues().SynthDialD + SpaceXS + 36
}

const compactDialLabelW = 58

// Three cells fit a desktop card, four a phone card, so a four-dial card keeps one row and a phone shows two cards.
const macroDialsMaxPerRow = 4

func macroDialsPerRow(n, cardW int) int {
	inner := cardW - 2*macroCardInset()
	per := (inner + SpaceSM) / (macroDialCellMinW() + SpaceSM)
	cap := macroDialsMaxPerRow
	if synthCompactCards() {
		// A compact card is sized to its dials (flow layout), so one row may hold them all.
		cap = synthdial.MaxDialsPerCard
	}
	return max(1, min(cap, min(n, per)))
}

func macroDialRows(n, cardW int) int {
	if n <= 0 {
		return 0
	}
	per := macroDialsPerRow(n, cardW)
	return (n + per - 1) / per
}

// A row that would fall off the card's bottom stays empty.
func macroCardDialRects(r image.Rectangle, n int) (out [synthdial.MaxDialsPerCard]image.Rectangle) {
	if n <= 0 {
		return out
	}
	n = min(n, synthdial.MaxDialsPerCard)
	inner := r.Inset(macroCardInset())
	per := macroDialsPerRow(n, r.Dx())
	w := (inner.Dx() - SpaceSM*(per-1)) / per
	if w <= 0 {
		return out
	}
	rowH := macroDialRowH()
	y0 := inner.Min.Y + macroCardTopH() + SpaceXS
	for i := 0; i < n; i++ {
		row, col := i/per, i%per
		y := y0 + row*(rowH+SpaceXS)
		if y+rowH > inner.Max.Y+2 {
			break
		}
		x := inner.Min.X + col*(w+SpaceSM)
		out[i] = image.Rect(x, y, x+w, y+rowH)
	}
	return out
}

// Half of a 270° sweep with the gap at the bottom: 0 lower left, 100 lower right.
const synthDialSweep = 3 * math.Pi / 4

// Visualizers colours, never the chrome accent.
func drawSynthMacroDial(dst *ebiten.Image, cell image.Rectangle, s macroDialSlot) {
	if cell.Empty() {
		return
	}
	if synthCompactCards() {
		drawSynthMacroKnob(dst, cell, s)
		return
	}
	d := Profile().DensityValues().SynthDialD
	if d > cell.Dy() {
		d = cell.Dy()
	}
	arcR := image.Rect(cell.Min.X, cell.Min.Y+(cell.Dy()-d)/2, cell.Min.X+d, cell.Min.Y+(cell.Dy()-d)/2+d)
	var trackCol color.Color = WithAlpha(TokenTextSecondary(), AlphaMedium)
	var valueCol color.Color = colConceptStroke
	var labelCol color.Color = TokenTextPrimary()
	var readCol color.Color = WithAlpha(TokenTextSecondary(), AlphaMedium)
	if s.dim {
		valueCol, labelCol, readCol = TokenTextDisabled(), TokenTextDisabled(), TokenTextDisabled()
	}
	cx := float32(arcR.Min.X) + float32(d)/2
	cy := float32(arcR.Min.Y) + float32(d)/2
	stroke := float32(d) / 9
	if stroke < 1.5 {
		stroke = 1.5
	}
	rad := float32(d)/2 - stroke
	top := -math.Pi / 2
	start := top - synthDialSweep
	strokeArc(dst, cx, cy, rad, start, top+synthDialSweep, stroke, trackCol)
	if s.reading > 0 {
		strokeArc(dst, cx, cy, rad, start, start+2*synthDialSweep*float64(s.reading)/100, stroke+0.5, valueCol)
	}

	tx := arcR.Max.X + SpaceXS
	tw := cell.Max.X - tx
	if tw < 8 {
		return
	}
	mh := StyledTextHeight(RoleMicro)
	ty := cell.Min.Y + (cell.Dy()-2*mh)/2
	DrawTextStyled(dst, truncCaptionStyled(s.label, tw, RoleMicro), tx, ty, RoleMicro, labelCol)
	DrawTextStyled(dst, synthDialReadingLabel(s.reading), tx, ty+mh, RoleMicro, readCol)
}

// Same arc geometry and colours as drawSynthMacroDial.
func drawSynthMacroKnob(dst *ebiten.Image, cell image.Rectangle, s macroDialSlot) {
	mh := StyledTextHeight(RoleMicro)
	d := Profile().DensityValues().SynthKnobD
	if d > cell.Dy()-mh-1 {
		d = cell.Dy() - mh - 1
	}
	if d < 8 {
		return
	}
	var trackCol color.Color = WithAlpha(TokenTextSecondary(), AlphaMedium)
	var valueCol color.Color = colConceptStroke
	var labelCol color.Color = TokenTextPrimary()
	var readCol color.Color = TokenTextSecondary()
	if s.dim {
		valueCol, labelCol, readCol = TokenTextDisabled(), TokenTextDisabled(), TokenTextDisabled()
	} else if s.focused {
		labelCol, readCol = colConceptStroke, TokenTextPrimary()
	}
	cx := float32(cell.Min.X) + float32(cell.Dx())/2
	cy := float32(cell.Min.Y) + float32(d)/2
	stroke := float32(d) / 10
	if stroke < 2 {
		stroke = 2
	}
	rad := float32(d)/2 - stroke
	top := -math.Pi / 2
	start := top - synthDialSweep
	strokeArc(dst, cx, cy, rad, start, top+synthDialSweep, stroke, trackCol)
	if s.reading > 0 {
		strokeArc(dst, cx, cy, rad, start, start+2*synthDialSweep*float64(s.reading)/100, stroke+0.5, valueCol)
	}
	read := synthDialReadingLabel(s.reading)
	rw := StyledTextWidth(read, RoleMicro)
	DrawTextStyled(dst, read, int(cx)-rw/2, int(cy)-mh/2, RoleMicro, readCol)
	label := truncCaptionStyled(s.label, cell.Dx(), RoleMicro)
	lw := StyledTextWidth(label, RoleMicro)
	DrawTextStyled(dst, label, int(cx)-lw/2, cell.Min.Y+d+1, RoleMicro, labelCol)
}

// 101 strings built once, so a steady frame allocates nothing.
func synthDialReadingLabel(r int) string { return synthDialReadingLabels[max(0, min(100, r))] }

var synthDialReadingLabels = func() (out [101]string) {
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out
}()

// strokeArc strokes a circular arc from a0 to a1 (radians, 0 = +x, +π/2 = down).
func strokeArc(dst *ebiten.Image, cx, cy, r float32, a0, a1 float64, w float32, col color.Color) {
	if a1 <= a0 || r <= 0 {
		return
	}
	steps := int((a1 - a0) / (math.Pi / 24))
	if steps < 2 {
		steps = 2
	}
	var p vector.Path
	for i := 0; i <= steps; i++ {
		a := a0 + (a1-a0)*float64(i)/float64(steps)
		x := cx + float32(math.Cos(a))*r
		y := cy + float32(math.Sin(a))*r
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	drawPath(dst, &p, makeStrokeOptions(w), col)
}
