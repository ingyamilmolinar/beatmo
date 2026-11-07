package ui

// Visualizers colours, never the chrome accent, derived in rebuildThemeAliases so a theme change repaints.
// The glow is drawn outside the sprite cache.

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	// Derived in rebuildThemeAliases (theme.go).
	colGraphLo       color.NRGBA // the control's low end (cool)
	colGraphHi       color.NRGBA // the control's high end (warm)
	colGraphBefore   color.NRGBA // the sound before the gesture
	colGraphDiff     color.NRGBA // where NOW and BEFORE differ
	colGraphGrid     color.NRGBA // grid lines
	colGraphPlotBg   color.NRGBA // the plot's wash
	colGraphGlow     color.NRGBA // the trigger glow
	colGraphGlowRing color.NRGBA // the glow's ring around the plot

	// Shared by every Synth-tab data surface; never the chrome accent.
	colConceptFill   color.NRGBA
	colConceptStroke color.NRGBA
)

func graphRoleColor(r plotRole) color.NRGBA {
	switch r {
	case roleLo:
		return colGraphLo
	case roleHi:
		return colGraphHi
	case roleBefore:
		return colGraphBefore
	}
	return colConceptStroke
}

// drawViewPlot paints vp with its local origin at off.
func drawViewPlot(dst *ebiten.Image, off image.Point, vp *viewPlot) {
	card := image.Rect(0, 0, vp.w, vp.h).Add(off)
	drawRoundedRect(dst, card, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, card, TokenBorderSubtle(), RadiusSM, false)
	mh := StyledTextHeight(RoleMicro)
	x0 := off.X + viewPad
	y0 := off.Y + viewPad/2
	titleW := 0
	if vp.title != "" {
		titleW = StyledTextWidth(vp.title, RoleMicro)
		tx := card.Max.X - viewPad - titleW - detailsToggleReserve()
		col := TokenTextSecondary()
		if vp.pickable {
			// The developer view picker: tap the title to cycle the view.
			const caretW = 7
			tx -= caretW + 2
			col = TokenTextPrimary()
			DrawIcon(dst, IconChevronRight, image.Rect(card.Max.X-viewPad-caretW, y0+1, card.Max.X-viewPad, y0+1+caretW), WithAlphaFromColor(genColorPrimary, AlphaStrong))
			titleW += caretW + 2
		}
		DrawTextStyled(dst, vp.title, tx, y0, RoleMicro, col)
	}
	if vp.caption != "" {
		DrawTextStyled(dst, truncCaptionStyled(vp.caption, vp.w-2*viewPad-titleW-SpaceSM-detailsToggleReserve(), RoleMicro), x0, y0, RoleMicro, colConceptStroke)
	}
	if vp.empty != "" {
		w := StyledTextWidth(vp.empty, RoleMicro)
		DrawTextStyled(dst, vp.empty, card.Min.X+(vp.w-w)/2, card.Min.Y+vp.h/2-mh/2, RoleMicro, TokenTextSecondary())
		return
	}
	chipW := 0
	if vp.chip != "" {
		chipW = StyledTextWidth(vp.chip, RoleMicro)
		DrawTextStyled(dst, vp.chip, card.Max.X-viewPad-chipW-detailsToggleReserve(), y0+mh+2, RoleMicro, TokenTextSecondary())
	}
	if line := vp.shownReadout(); line != "" {
		DrawTextStyled(dst, truncCaptionStyled(line, vp.w-2*viewPad-chipW-SpaceSM-detailsToggleReserve(), RoleMicro), x0, y0+mh+2, RoleMicro, TokenTextPrimary())
	}
	plot := vp.plot.Add(off)
	if plot.Dx() < 4 || plot.Dy() < 4 {
		return
	}
	drawRect(dst, plot, colGraphPlotBg, true)
	for _, t := range vp.yTicks {
		y := off.Y + int(t.pos)
		drawRect(dst, image.Rect(plot.Min.X, y, plot.Max.X, y+1), colGraphGrid, true)
		if t.label != "" {
			w := StyledTextWidth(t.label, RoleMicro)
			DrawTextStyled(dst, t.label, plot.Min.X-3-w, y-mh/2, RoleMicro, TokenTextSecondary())
		}
	}
	for _, t := range vp.xTicks {
		x := off.X + int(t.pos)
		if t.minor {
			drawRect(dst, image.Rect(x, plot.Max.Y-3, x+1, plot.Max.Y), colGraphGrid, true)
			continue
		}
		drawRect(dst, image.Rect(x, plot.Min.Y, x+1, plot.Max.Y), colGraphGrid, true)
		if t.label != "" {
			w := StyledTextWidth(t.label, RoleMicro)
			lx := min(max(x-w/2, plot.Min.X-viewPad), card.Max.X-viewPad-w)
			DrawTextStyled(dst, t.label, lx, plot.Max.Y+1, RoleMicro, TokenTextSecondary())
		}
	}
	for _, b := range vp.diff {
		fillPlotBand(dst, b, off, colGraphDiff)
	}
	for _, b := range vp.bands {
		fillPlotBand(dst, b, off, colConceptFill)
	}
	// Ghosts first, NOW on top.
	for pass := 0; pass < 2; pass++ {
		for _, l := range vp.lines {
			if (l.role == roleNow) != (pass == 1) {
				continue
			}
			w := float32(1.5)
			if l.role == roleNow {
				w = 2
			}
			strokePlotLine(dst, l.pts, off, w, graphRoleColor(l.role))
		}
	}
	for _, m := range vp.markers {
		x := off.X + int(m.x)
		col := graphRoleColor(m.role)
		col.A /= 2
		drawRect(dst, image.Rect(x, plot.Min.Y, x+1, plot.Max.Y), col, true)
	}
	// One geometry with placeLabels (axisWordRects), so these spots stay clear of the ghost labels.
	for _, t := range vp.axisWordRects() {
		DrawTextStyled(dst, t.text, off.X+t.r.Min.X, off.Y+t.r.Min.Y, RoleMicro, WithAlpha(TokenTextSecondary(), AlphaStrong))
	}
	for _, l := range vp.labels {
		DrawTextStyled(dst, l.text, off.X+int(l.x), off.Y+int(l.y), RoleMicro, graphRoleColor(l.role))
	}
}

// Baked once per picture and theme: a glowing frame only blits it with scaled alpha. With a beat playing the
// glow is on almost every frame, so per-frame tessellation is out.
type glowLayer struct {
	img    *ebiten.Image
	vp     *viewPlot
	gen    int
	op     ebiten.DrawImageOptions
	builds int // bakes so far (tests)
}

// draw paints the glow over a drawn plot at intensity 0..1.
func (g *glowLayer) draw(dst *ebiten.Image, off image.Point, vp *viewPlot, intensity float64) {
	if vp == nil || intensity <= 0 || vp.empty != "" {
		return
	}
	if g.vp != vp || g.gen != ThemeGeneration() || g.img == nil {
		if g.img == nil || g.img.Bounds().Dx() != vp.w || g.img.Bounds().Dy() != vp.h {
			g.release()
			g.img = ebiten.NewImage(vp.w, vp.h)
		} else {
			g.img.Clear()
		}
		for _, l := range vp.lines {
			if l.role == roleNow {
				strokePlotLine(g.img, l.pts, image.Point{}, 4, colGraphGlow)
			}
		}
		drawRoundedRect(g.img, vp.plot.Inset(-1), colGraphGlowRing, RadiusControl, false)
		g.vp, g.gen = vp, ThemeGeneration()
		g.builds++
	}
	g.op = ebiten.DrawImageOptions{}
	g.op.GeoM.Translate(float64(off.X), float64(off.Y))
	k := float32(clamp01(intensity))
	g.op.ColorScale.Scale(k, k, k, k) // premultiplied: a fade by k
	dst.DrawImage(g.img, &g.op)
}

func (g *glowLayer) release() {
	if g.img != nil {
		g.img.Deallocate()
		g.img = nil
	}
	g.vp = nil
}

func strokePlotLine(dst *ebiten.Image, pts []fpt, off image.Point, w float32, col color.Color) {
	if len(pts) < 2 {
		return
	}
	var p vector.Path
	ox, oy := float32(off.X), float32(off.Y)
	p.MoveTo(pts[0].x+ox, pts[0].y+oy)
	for _, q := range pts[1:] {
		p.LineTo(q.x+ox, q.y+oy)
	}
	drawPath(dst, &p, makeStrokeOptions(w), col)
}

// One column at a time: seam-free under a translucent colour (a polygon fill showed its triangle fan) and
// visible to drawRect-intercepting pixel tests.
func fillPlotBand(dst *ebiten.Image, b plotBand, off image.Point, col color.Color) {
	if len(b.xs) < 2 {
		return
	}
	j := 0
	for x := int(b.xs[0] + 0.5); float32(x) <= b.xs[len(b.xs)-1]; x++ {
		fx := float32(x)
		for j < len(b.xs)-2 && b.xs[j+1] < fx {
			j++
		}
		x0, x1 := b.xs[j], b.xs[j+1]
		f := float32(0)
		if x1 > x0 {
			f = (fx - x0) / (x1 - x0)
		}
		f = minf(1, maxf(0, f))
		t := b.top[j] + f*(b.top[j+1]-b.top[j])
		bo := b.bot[j] + f*(b.bot[j+1]-b.bot[j])
		if bo < t {
			t, bo = bo, t
		}
		y0, y1 := int(t+0.5), int(bo+0.5)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		drawRect(dst, image.Rect(off.X+x, off.Y+y0, off.X+x+1, off.Y+y1), col, true)
	}
}

func toNRGBA(c color.Color) color.NRGBA { return color.NRGBAModel.Convert(c).(color.NRGBA) }
