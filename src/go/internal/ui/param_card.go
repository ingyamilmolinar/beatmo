// src/go/internal/ui/param_card.go
package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// ParamCard is the closed state of every synth/sampler parameter control: name, resolution-aware value
// and a thin range gauge. A tap opens the wheel popup; on desktop the mouse wheel nudges in place.

type paramCardGaugeKind int

const (
	gaugeFill paramCardGaugeKind = iota
	gaugeBipolar
	gaugeDots
)

// Longer enums fall back to an index-proportional fill.
const paramCardMaxDots = 8

type paramCardGauge struct {
	kind  paramCardGaugeKind
	frac  float64
	zero  float64
	count int
	idx   int
}

type paramCardState struct{ Selected, Dim bool }

// paramCardGaugeFor derives the gauge from the shared knob value model.
func paramCardGaugeFor(k *Knob, def audio.ParamDef) paramCardGauge {
	v := 0.0
	if k != nil {
		v = k.Value
	}
	if n := len(def.Enum); n > 0 && n <= paramCardMaxDots {
		span := def.Max - def.Min
		actual := def.Min + v*span
		idx := int(math.Round(actual))
		if idx < 0 {
			idx = 0
		} else if idx >= n {
			idx = n - 1
		}
		return paramCardGauge{kind: gaugeDots, count: n, idx: idx}
	}
	if k != nil && k.Bipolar {
		return paramCardGauge{kind: gaugeBipolar, frac: v, zero: k.ZeroFrac}
	}
	return paramCardGauge{kind: gaugeFill, frac: v}
}

// drawParamCard renders the card. Token accessors only; no-op on empty rect.
func drawParamCard(dst *ebiten.Image, r image.Rectangle, name, value string, g paramCardGauge, st paramCardState) {
	if r.Empty() {
		return
	}
	// A peek-band card can be a few px tall, so fit the radius to the rect.
	rad := fitRadius(RadiusControl, r)
	// Flat-tonal: one filled body; the silhouette is the surface-3 tone step, and selection a 2 px accent bar.
	capR := r
	drawFlatButtonBody(dst, capR, TokenSurface3(), rad)
	if st.Selected {
		drawFlatSelectionBar(dst, capR, colConceptStroke, selectionEdgeBottom, 1)
	}

	inner := capR.Inset(SpaceXS + 2)
	if inner.Empty() {
		return
	}
	nameCol := TokenTextSecondary()
	valCol := colTextPrimary
	if st.Dim {
		nameCol = TokenTextDisabled()
		valCol = TokenTextDisabled()
	}
	dv2 := Profile().DensityValues()
	gaugeH := dv2.ParamCardGaugeH
	nameH := StyledTextHeight(RoleMicro)
	valH := StyledTextHeight(RoleBody)

	// Name row (top).
	nm := truncCaptionStyled(name, inner.Dx(), RoleMicro)
	DrawTextStyled(dst, nm, inner.Min.X, inner.Min.Y, RoleMicro, nameCol)

	// Value row (center between name and gauge) + "opens" caret at right.
	const caretW = 9
	valTop := inner.Min.Y + nameH
	valBottom := inner.Max.Y - gaugeH - SpaceXS
	vy := valTop + (valBottom-valTop-valH)/2
	if vy < valTop {
		vy = valTop
	}
	vzW := inner.Dx() - caretW - SpaceXS
	val := truncCaptionStyled(value, vzW, RoleBody)
	DrawTextStyled(dst, val, inner.Min.X, vy+1, RoleBody, WithAlphaFromColor(color.Black, 150))
	DrawTextStyled(dst, val, inner.Min.X, vy, RoleBody, valCol)
	cy := vy + valH/2
	cx := inner.Max.X - caretW
	DrawIcon(dst, IconChevronDown, image.Rect(cx, cy-caretW/2, cx+caretW, cy+caretW/2+1),
		WithAlphaFromColor(genColorPrimary, AlphaStrong))

	// Gauge strip (bottom).
	gr := image.Rect(inner.Min.X, inner.Max.Y-gaugeH, inner.Max.X, inner.Max.Y)
	drawParamCardGauge(dst, gr, g, st.Dim)
}

func drawParamCardGauge(dst *ebiten.Image, gr image.Rectangle, g paramCardGauge, dim bool) {
	var fill color.Color = genColorPrimary
	if dim {
		fill = TokenTextDisabled()
	}
	drawGaugeStrip(dst, gr, g, fill)
}

// The one renderer for every valued control's gauge strip; only the fill color varies.
func drawGaugeStrip(dst *ebiten.Image, gr image.Rectangle, g paramCardGauge, fill color.Color) {
	if gr.Empty() {
		return
	}
	track := WithAlpha(TokenSurface1(), AlphaStrong)
	drawCapsule(dst, gr, track, true)
	switch g.kind {
	case gaugeDots:
		if g.count <= 0 {
			return
		}
		d := gr.Dy()
		stride := 0
		if g.count > 1 {
			stride = (gr.Dx() - d) / (g.count - 1)
		}
		for i := 0; i < g.count; i++ {
			x := gr.Min.X + i*stride
			dot := image.Rect(x, gr.Min.Y, x+d, gr.Max.Y)
			var col color.Color = WithAlphaFromColor(fill, AlphaSubtle)
			if i == g.idx {
				col = fill
			}
			drawCapsule(dst, dot, col, true)
		}
	case gaugeBipolar:
		zx := gr.Min.X + int(g.zero*float64(gr.Dx()))
		vx := gr.Min.X + int(clamp01(g.frac)*float64(gr.Dx()))
		seg := image.Rect(zx, gr.Min.Y, vx, gr.Max.Y)
		if vx < zx {
			seg = image.Rect(vx, gr.Min.Y, zx, gr.Max.Y)
		}
		if seg.Dx() < 2 { // always show the center detent
			seg.Max.X = seg.Min.X + 2
		}
		drawCapsule(dst, seg, fill, true)
	default: // gaugeFill
		w := int(clamp01(g.frac) * float64(gr.Dx()))
		if w < 2 && g.frac > 0 {
			w = 2
		}
		if w <= 0 {
			return // an empty level is the track alone — no zero-width draw
		}
		drawCapsule(dst, image.Rect(gr.Min.X, gr.Min.Y, gr.Min.X+w, gr.Max.Y), fill, true)
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
