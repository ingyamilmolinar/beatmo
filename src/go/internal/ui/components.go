package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawEdgeLine is defined as a variable so tests can intercept edge rendering
// and verify arrowhead behaviour.
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
	// Outer glow — radius multiplier sourced from DESIGN.md
	// geometry.signal-glow-radius-multiplier.
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

// drawNodeBloom paints a soft, multi-ring "Vice-City" neon bloom in the node's
// own instrument color, centered on (cx,cy) screen pixels, sized off the node's
// screen radius rPx. The bloom is three concentric *filled* rounded rects drawn
// back-to-front — widest/faintest first, tightest/brightest last — so the
// alpha-blended falloff reads as a radial halo rather than a hard box. Each
// ring uses an EXISTING alpha bucket via WithAlphaFromColor: AlphaFaint /
// AlphaMedium / AlphaStrong. Even the pink/magenta instrument family reads
// because the innermost ring lands at AlphaStrong (180).
//
// intensity (0..1) is the live node-fire animation level; it scales each ring's
// alpha so the bloom decays naturally as the firing animation decays, and it
// scales the outer-ring spread so a freshly-fired node blooms wider than a
// fading one. The most-recently-fired node therefore carries the brightest,
// widest bloom automatically (latest-hit emphasis).
//
// The two wider outer rings are gated on rPx >= 8 so tiny zoomed-out nodes
// don't over-bloom — at small radii only the tight inner ring draws, preserving
// a visible firing cue without a runaway box. The spread is capped so the
// widest ring never extends more than ~7px past the node radius, keeping the
// bloom inside the bound asserted by TestDesktopNodeHighlightReasonableSize
// (<= 2*rPx+16 near the node).
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
	// Ring insets past the node radius (px). The widest ring (idx 0) spreads
	// with intensity so a fresh hit blooms further than a fading one, capped at
	// +7px (so width <= 2*rPx+14 < 2*rPx+16, inside the highlight-size bound).
	spread := int(math.Round(4 + 3*intensity)) // 4..7
	insets := [3]int{spread, spread - 3, 1}
	alphas := [3]uint8{
		scaleAlpha(AlphaFaint, intensity),
		scaleAlpha(AlphaMedium, intensity),
		scaleAlpha(AlphaStrong, intensity),
	}
	// Inner (brightest) ring always draws; the two wider halo rings are gated
	// on a comfortable on-screen radius so tiny nodes don't over-bloom.
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

// drawRectHoverGlow paints an own-color hover highlight that CONFORMS to the
// component rect r for ANY aspect ratio — a tall thin line, a wide pill, or a
// square. It assumes NO shape: every stroke derives from r itself, so the
// highlight follows the component's real bounds instead of a centred blob (the
// failure mode of drawNodeBloom on a non-square control). Composed of:
//
//   - a few concentric 1px outlines expanding outward from r with alpha falloff
//     (an edge-hugging glow that traces r at every step),
//   - a soft own-color fill over r's interior (covers the component body), and
//   - a crisp own-color outline hugging r's edge.
//
// This is the shared hover-highlight primitive for every OWN-COLOR glow — grid
// nodes, EQ band/HP/LP handles, sampler trim lines — so they all behave
// identically for any shape. (drawNodeBloom is kept only for the tuned neon
// FIRING bloom.) p (0..1) scales the whole effect.
func drawRectHoverGlow(dst *ebiten.Image, r image.Rectangle, col color.Color, p float64) {
	if dst == nil || col == nil || r.Empty() || p <= 0 {
		return
	}
	if p > 1 {
		p = 1
	}
	// Outward glow — concentric 1px outlines expanding from r, brightest at the
	// edge and fading with distance. Each traces r's actual shape.
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

// DrawProgress renders a portion of the edge according to progress t
// (0..1). When t reaches 1 the arrow head is drawn.
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
	// live, when set, is consulted at draw time instead of Fill so a style
	// COPIED at construction (`b.Style = InstButtonStyle`, profile fields)
	// still follows a runtime theme applied later. ButtonStyleFromSpec and
	// the hand-composed theme.go styles set it; Fill stays populated for
	// callers that read the value directly. A small comparable struct (not
	// a func) so styles stay `==`-comparable (menus test Style identity).
	live liveFill
	// Radius, when > 0, draws the button as a rounded rectangle using
	// drawRoundedButton instead of the square drawButton path. Use a
	// RadiusSM / RadiusMD token from touch_sizes.go. Zero preserves the
	// historical square chrome so all existing ButtonStyle values are
	// unaffected by this field addition.
	Radius int
}

// ButtonStyleFromSpec builds a ButtonStyle from a generated ComponentSpec
// looked up by ID. Used by theme.go to source the legacy *Style vars from
// DESIGN.md instead of hand-coded color literals — every theme.go var that
// has a corresponding ComponentID can flip its initialiser to this one
// call. Behaviour stays byte-equivalent (TestComponentSpecsDrift verifies
// the spec values match the original literals).
//
// Returning a value (not a pointer) preserves the historical struct-copy
// semantics of theme.go assignments. The Border color flows through as
// the BorderRef.Resolve() result — concrete NRGBA so existing call sites
// receive the same type they did before.
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

// liveFill is the comparable "how to re-read my fill" tag carried by
// ButtonStyle / TextInputStyle copies.
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

// Draw renders the button rectangle. When Radius > 0 the button is drawn as a
// rounded rectangle via drawRoundedButton; otherwise delegates to renderLegacy
// (the chrome-rendering primitive in render.go) so all chrome flows through a
// single path. Hover/press deltas mirror the historical hand-coded values
// that Phase 2 captured byte-equivalent in DESIGN.md `button-secondary`
// (verified by TestComponentSpecsDrift).
func (s ButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	// Flat-tonal (2026-07-25): one filled body, no border. s.Border is kept on
	// the struct (call sites and specs still set it) but is no longer painted —
	// a control's silhouette is the tone step against its container. Hover and
	// press remain the historical +12 / -20 brightness deltas so the
	// interaction FEEL is unchanged; only the geometry is.
	// See DESIGN.md "Elevation & Depth".
	drawFlatButtonBody(dst, r, flatStateFill(s.fill(), ComponentState{Pressed: pressed, Hovered: hovered}), fitRadius(s.Radius, r))
}

// DrawAnimated draws the button with a shrink animation controlled by anim
// (0..1). anim is typically set to 1 on click and decays toward 0.
func (s ButtonStyle) DrawAnimated(dst *ebiten.Image, r image.Rectangle, pressed bool, anim float64) {
	if anim < 0 {
		anim = 0
	}
	inset := int(anim * float64(r.Dx()) * 0.1)
	animRect := image.Rect(r.Min.X+inset, r.Min.Y+inset, r.Max.X-inset, r.Max.Y-inset)
	drawButton(dst, animRect, s.fill(), s.Border, pressed, Profile().DrawTopEdgeHighlight)
}

// ColorSwatchStyle draws a solid color swatch button whose fill is provided
// dynamically via a function. This is used for per-row color selection where
// each button reflects the current row color.
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
	// Swatch hover delta is +20 (vs ButtonStyle's +12) — historical
	// chrome decision preserved verbatim. Border has no hover delta.
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
	// live: re-resolve the fill at draw time for styles copied at
	// construction (see ButtonStyle.live).
	live liveFill
}

func (s TextInputStyle) fill() color.Color { return s.live.resolve(s.Fill) }

// TextInputStyleFromSpec builds a TextInputStyle from a generated
// ComponentSpec. Cursor color is hand-passed because cursor-color isn't
// a per-component design token (it's universally white across inputs);
// future generator support could add a `cursorColor:` schema field.
func TextInputStyleFromSpec(id ComponentID, cursor color.Color) TextInputStyle {
	spec := Spec(id)
	return TextInputStyle{
		Fill:   spec.Fill,
		Border: spec.Border.Resolve(),
		Cursor: cursor,
		live:   liveFill{kind: liveFillSpec, spec: id},
	}
}

// Draw renders the text box background. The pressed flag represents focus
// in the legacy API; we map it to ComponentState.Pressed for the
// renderLegacy primitive (same drawButton effect). No hover/focus deltas
// are applied in the static path — only DrawAnimated handles those.
func (s TextInputStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	// Flat-tonal: an input shares the CONTROL body (the input-field spec
	// fill, the same surface-2 tone as every keycap) so a label box and the
	// stepper buttons beside it read as one component family. The focus ring
	// (drawn by DrawAnimated) is what marks it as a slot you type into.
	// See DESIGN.md "Elevation & Depth". Guard:
	// TestVolumeButtonsAndInputsShareKeycapBody.
	drawFlatButtonBody(dst, r, s.fill(), fitRadius(RadiusControl, r))
}

// DrawAnimated draws the text box with a subtle focus animation. The
// focus delta (+30 fill / +80 border) and accent ring are produced by
// renderLegacy; the inset animation rectangle is computed here and
// passed through.
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
	// Flat-tonal: the shared control body (spec fill), brightened while
	// focused. The hot-pink focus ring is the one outline the system still
	// draws.
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

// narrowCellThreshold is the cell width (px) at or below which borders are
// suppressed to keep active cells visible at high subdivisions.
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
		// Normal path: flat fill + full border (no lit top strip — DESIGN.md
		// Rule 2, nothing is 3D).
		drawRect(dst, r, s.Border, false)
	} else if on || highlighted {
		// Narrow active cell: right-edge separator only (1px vs 2px)
		drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), s.Border, true)
	}
	// Narrow inactive: no border — row stripe background provides separation
}

// DrumRowStyle is reserved for future customisation of entire rows.
type DrumRowStyle struct{}
