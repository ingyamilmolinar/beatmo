package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// A var so tests can intercept edge rendering.
var drawEdgeLine = DrawLineCam

// pixel helper is defined in widgets.go and reused here.

// fadeColor returns c with its alpha scaled by t (0..1).
func fadeColor(c color.Color, t float64) color.Color {
	r, g, b, a := c.RGBA()
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(float64(a>>8) * t)}
}

// NodeStyle defines visual appearance for graph nodes.
type NodeStyle struct {
	Radius float32
	Fill   color.Color
	Border color.Color
}

// Draw renders a node at world coordinates using the provided camera matrix.
func (s NodeStyle) Draw(dst *ebiten.Image, x, y float64, cam *ebiten.GeoM) {
	size := float64(s.Radius) * 2
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(size, size)
	op.GeoM.Translate(x-float64(s.Radius), y-float64(s.Radius))
	op.GeoM.Concat(*cam)
	dst.DrawImage(pixel(s.Fill), &op)
	thk := float64(genGeomNodeBorderThickness)
	DrawLineCam(dst, x-float64(s.Radius), y-float64(s.Radius), x+float64(s.Radius), y-float64(s.Radius), cam, s.Border, thk)
	DrawLineCam(dst, x+float64(s.Radius), y-float64(s.Radius), x+float64(s.Radius), y+float64(s.Radius), cam, s.Border, thk)
	DrawLineCam(dst, x+float64(s.Radius), y+float64(s.Radius), x-float64(s.Radius), y+float64(s.Radius), cam, s.Border, thk)
	DrawLineCam(dst, x-float64(s.Radius), y+float64(s.Radius), x-float64(s.Radius), y-float64(s.Radius), cam, s.Border, thk)
}

// SignalStyle defines the appearance of travelling pulses between nodes.
type SignalStyle struct {
	Radius float32
	Color  color.Color
}

// Draw renders the signal at world coordinates with the given camera transform.
func (s SignalStyle) Draw(dst *ebiten.Image, x, y float64, cam *ebiten.GeoM) {
	size := float64(s.Radius) * 2
	var op ebiten.DrawImageOptions
	glowSize := size * genGeomSignalGlowRadiusMultiplier
	op.GeoM.Scale(glowSize, glowSize)
	op.GeoM.Translate(x-glowSize/2, y-glowSize/2)
	op.GeoM.Concat(*cam)
	dst.DrawImage(pixel(fadeColor(s.Color, float64(genAnimSignalGlowOuter))), &op)

	// Inner core
	var op2 ebiten.DrawImageOptions
	op2.GeoM.Scale(size, size)
	op2.GeoM.Translate(x-size/2, y-size/2)
	op2.GeoM.Concat(*cam)
	dst.DrawImage(pixel(s.Color), &op2)
}

// Three filled rings back to front, so the falloff reads as a halo rather than a box; intensity scales alpha
// and spread so the latest hit blooms brightest. Small nodes get the inner ring only; the spread stays under the
// highlight-size bound.
func drawNodeBloom(dst *ebiten.Image, cx, cy, rPx int, nodeColor color.Color, intensity float64) {
	if dst == nil || nodeColor == nil {
		return
	}
	if intensity < 0 {
		intensity = 0
	}
	if intensity > 1 {
		intensity = 1
	}
	if rPx < 1 {
		rPx = 1
	}
	// Capped at +7 px so the width stays under 2·rPx+16.
	spread := int(math.Round(4 + 3*intensity)) // 4..7
	insets := [3]int{spread, spread - 3, 1}
	alphas := [3]uint8{
		scaleAlpha(AlphaFaint, intensity),
		scaleAlpha(AlphaMedium, intensity),
		scaleAlpha(AlphaStrong, intensity),
	}
	// Tiny nodes get only the inner ring, so they never over-bloom.
	start := 2
	if rPx >= 8 {
		start = 0
	}
	for i := start; i < 3; i++ {
		ins := insets[i]
		if ins < 1 {
			ins = 1
		}
		ext := rPx + ins
		col := WithAlphaFromColor(nodeColor, alphas[i])
		if col.A == 0 {
			continue
		}
		r := image.Rect(cx-ext, cy-ext, cx+ext, cy+ext)
		// Corner radius ~half the half-extent gives a rounded, blob-like ring.
		drawRoundedRect(dst, r, col, ext/2, true)
	}
}

// The one own-color hover glow for any shape: every stroke derives from r, so it follows a thin line or a pill
// instead of a centred blob. drawNodeBloom stays only for the firing bloom.
func drawRectHoverGlow(dst *ebiten.Image, r image.Rectangle, col color.Color, p float64) {
	if dst == nil || col == nil || r.Empty() || p <= 0 {
		return
	}
	if p > 1 {
		p = 1
	}
	const spread = 4
	for i := spread; i >= 1; i-- {
		frac := float64(spread-i+1) / float64(spread+1) // nearer the edge → brighter
		if a := scaleAlpha(AlphaStrong, p*frac); a > 0 {
			drawRect(dst, r.Inset(-i), WithAlphaFromColor(col, a), false)
		}
	}
	// Soft interior fill (covers the body) + crisp edge ring.
	if a := scaleAlpha(90, p); a > 0 {
		drawRect(dst, r, WithAlphaFromColor(col, a), true)
	}
	if a := scaleAlpha(235, p); a > 0 {
		drawRect(dst, r, WithAlphaFromColor(col, a), false)
	}
}

// scaleAlpha multiplies an alpha bucket value by t (0..1), clamped.
func scaleAlpha(a uint8, t float64) uint8 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return a
	}
	return uint8(math.Round(float64(a) * t))
}

// EdgeStyle draws directional edges between nodes.
type EdgeStyle struct {
	Color     color.Color
	Thickness float64
	ArrowSize float64
}

// Draw renders a directed edge from (x1,y1) to (x2,y2) using cam.
func (s EdgeStyle) Draw(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM) {
	s.DrawProgress(dst, x1, y1, x2, y2, cam, 1)
}

// The arrowhead draws at t = 1.
func (s EdgeStyle) DrawProgress(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM, t float64) {
	if t <= 0 {
		return
	}
	if t > 1 {
		t = 1
	}
	col := fadeColor(s.Color, t)
	ex := x1 + (x2-x1)*t
	ey := y1 + (y2-y1)*t
	drawEdgeLine(dst, x1, y1, ex, ey, cam, col, s.Thickness)
	if t < 1 {
		return
	}
	angle := math.Atan2(y2-y1, x2-x1)

	// Arrowhead at the end of the edge
	leftX := x2 - s.ArrowSize*math.Cos(angle-math.Pi/6)
	leftY := y2 - s.ArrowSize*math.Sin(angle-math.Pi/6)
	rightX := x2 - s.ArrowSize*math.Cos(angle+math.Pi/6)
	rightY := y2 - s.ArrowSize*math.Sin(angle+math.Pi/6)
	drawEdgeLine(dst, x2, y2, leftX, leftY, cam, col, s.Thickness)
	drawEdgeLine(dst, x2, y2, rightX, rightY, cam, col, s.Thickness)
}

// ButtonStyle describes rectangular button visuals.
type ButtonStyle struct {
	Fill   color.Color
	Border color.Color
	// Re-read at draw time so a style COPIED at construction follows a later runtime theme; a comparable
	// struct, not a func, so styles stay ==-comparable.
	live liveFill
	// 0 keeps the square chrome existing styles rely on.
	Radius int
}

// Returns a value to keep theme.go's struct-copy semantics; Border is the concrete resolved NRGBA.
func ButtonStyleFromSpec(id ComponentID) ButtonStyle {
	spec := Spec(id)
	return ButtonStyle{Fill: spec.Fill, Border: spec.Border.Resolve(), live: liveFill{kind: liveFillSpec, spec: id}}
}

// liveFillKind names how a copied style re-resolves its fill at draw time.
type liveFillKind uint8

const (
	liveFillNone   liveFillKind = iota // use the captured Fill
	liveFillSpec                       // Spec(spec).Fill
	liveFillAccent                     // colAccent (mobile primary action)
)

type liveFill struct {
	kind liveFillKind
	spec ComponentID
}

func (l liveFill) resolve(captured color.Color) color.Color {
	switch l.kind {
	case liveFillSpec:
		return Spec(l.spec).Fill
	case liveFillAccent:
		return colAccent
	}
	return captured
}

// fill returns the live fill or the captured Fill.
func (s ButtonStyle) fill() color.Color { return s.live.resolve(s.Fill) }

// withLive returns a copy of s tagged to re-resolve its fill at draw time.
func (s ButtonStyle) withLive(kind liveFillKind) ButtonStyle {
	s.live = liveFill{kind: kind, spec: s.live.spec}
	s.Fill = s.fill()
	return s
}

func (s ButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	// Flat-tonal: one filled body; s.Border is kept on the struct but no longer painted. The +12/-20 deltas keep
	// the interaction feel unchanged.
	drawFlatButtonBody(dst, r, flatStateFill(s.fill(), ComponentState{Pressed: pressed, Hovered: hovered}), fitRadius(s.Radius, r))
}

// anim is typically 1 on click, decaying toward 0.
func (s ButtonStyle) DrawAnimated(dst *ebiten.Image, r image.Rectangle, pressed bool, anim float64) {
	if anim < 0 {
		anim = 0
	}
	inset := int(anim * float64(r.Dx()) * 0.1)
	animRect := image.Rect(r.Min.X+inset, r.Min.Y+inset, r.Max.X-inset, r.Max.Y-inset)
	drawButton(dst, animRect, s.fill(), s.Border, pressed, Profile().DrawTopEdgeHighlight)
}

type ColorSwatchStyle struct {
	Color  func() color.Color
	Border color.Color
}

func (s ColorSwatchStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	col := color.Color(genColorRowRackColorFallback)
	if s.Color != nil {
		if c := s.Color(); c != nil {
			col = c
		}
	}
	// +20, not ButtonStyle's +12, deliberately; the border has no hover delta.
	renderLegacy(dst, r, col, s.Border,
		InteractionDelta{FillDelta: 20},
		InteractionDelta{},
		ComponentState{Pressed: pressed, Hovered: hovered})
}

// TextInputStyle styles a text input box.
type TextInputStyle struct {
	Fill   color.Color
	Border color.Color
	Cursor color.Color
	// Re-resolved at draw time, as ButtonStyle.live.
	live liveFill
}

func (s TextInputStyle) fill() color.Color { return s.live.resolve(s.Fill) }

// The cursor colour is not a per-component token (it is white everywhere), so it is passed in.
func TextInputStyleFromSpec(id ComponentID, cursor color.Color) TextInputStyle {
	spec := Spec(id)
	return TextInputStyle{
		Fill:   spec.Fill,
		Border: spec.Border.Resolve(),
		Cursor: cursor,
		live:   liveFill{kind: liveFillSpec, spec: id},
	}
}

// No hover/focus deltas here; only DrawAnimated applies them.
func (s TextInputStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	// Shares the control body (the surface-2 tone) so a label box and its stepper buttons read as one family; the
	// focus ring marks it as a slot you type into.
	drawFlatButtonBody(dst, r, s.fill(), fitRadius(RadiusControl, r))
}

func (s TextInputStyle) DrawAnimated(dst *ebiten.Image, r image.Rectangle, focused bool, anim float64) {
	if anim < 0 {
		anim = 0
	}
	minDim := r.Dx()
	if r.Dy() < minDim {
		minDim = r.Dy()
	}
	inset := int(anim * float64(minDim) * 0.1)
	animRect := image.Rect(r.Min.X+inset, r.Min.Y+inset, r.Max.X-inset, r.Max.Y-inset)
	// The focus ring is the one outline the system still draws.
	fill := s.fill()
	if focused {
		fill = adjustColor(fill, 30)
	}
	drawFlatButtonBody(dst, animRect, fill, fitRadius(RadiusControl, animRect))
	if focused {
		drawRect(dst, animRect.Inset(1), WithAlpha(genColorFocusRing, genAlphaStrong), false)
	}
}

// DrumCellStyle styles individual drum machine cells.
type DrumCellStyle struct {
	On        color.Color
	Off       color.Color
	Highlight color.Color
	Border    color.Color
}

// Borders vanish at or below this so active cells stay visible at high subdivisions.
const narrowCellThreshold = 6

// Draw renders a drum cell considering its state. onCol overrides the default On color.
func (s DrumCellStyle) Draw(dst *ebiten.Image, r image.Rectangle, on, highlighted bool, onCol color.Color) {
	fill := s.Off
	if on {
		if onCol != nil {
			fill = onCol
		} else {
			fill = s.On
		}
	}
	if highlighted {
		fill = s.Highlight
	}

	narrow := r.Dx() <= narrowCellThreshold

	drawRect(dst, r, fill, true)

	if !narrow {
		// No lit top strip: nothing is 3D.
		drawRect(dst, r, s.Border, false)
	} else if on || highlighted {
		// Narrow active cell: right-edge separator only (1px vs 2px)
		drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), s.Border, true)
	}
	// Narrow inactive: no border — row stripe background provides separation
}

// DrumRowStyle is reserved for future customisation of entire rows.
type DrumRowStyle struct{}
