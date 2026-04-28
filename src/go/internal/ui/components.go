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
	return ButtonStyle{Fill: spec.Fill, Border: spec.Border.Resolve()}
}

// Draw renders the button rectangle. Delegates to renderLegacy (the
// chrome-rendering primitive in render.go) so all chrome flows through a
// single path. Hover/press deltas mirror the historical hand-coded values
// that Phase 2 captured byte-equivalent in DESIGN.md `button-secondary`
// (verified by TestComponentSpecsDrift).
func (s ButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	renderLegacy(dst, r, s.Fill, s.Border,
		InteractionDelta{FillDelta: 12, BorderDelta: 20}, // hover
		InteractionDelta{},                                 // focus (unused by buttons)
		ComponentState{Pressed: pressed, Hovered: hovered})
}

// DrawAnimated draws the button with a shrink animation controlled by anim
// (0..1). anim is typically set to 1 on click and decays toward 0.
func (s ButtonStyle) DrawAnimated(dst *ebiten.Image, r image.Rectangle, pressed bool, anim float64) {
	if anim < 0 {
		anim = 0
	}
	inset := int(anim * float64(r.Dx()) * 0.1)
	animRect := image.Rect(r.Min.X+inset, r.Min.Y+inset, r.Max.X-inset, r.Max.Y-inset)
	drawButton(dst, animRect, s.Fill, s.Border, pressed, Profile().DrawTopEdgeHighlight)
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
}

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
	}
}

// Draw renders the text box background. The pressed flag represents focus
// in the legacy API; we map it to ComponentState.Pressed for the
// renderLegacy primitive (same drawButton effect). No hover/focus deltas
// are applied in the static path — only DrawAnimated handles those.
func (s TextInputStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	renderLegacy(dst, r, s.Fill, s.Border,
		InteractionDelta{}, InteractionDelta{},
		ComponentState{Pressed: pressed, Hovered: hovered})
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
	renderLegacy(dst, animRect, s.Fill, s.Border,
		InteractionDelta{},                                 // hover (unused by inputs)
		InteractionDelta{FillDelta: 30, BorderDelta: 80},   // focus
		ComponentState{Focused: focused})
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
		// Normal path: top strip + full border
		if (on || highlighted) && r.Dy() > 4 {
			topStrip := image.Rect(r.Min.X+1, r.Min.Y, r.Max.X-1, r.Min.Y+1)
			drawRect(dst, topStrip, adjustColor(fill, 30), true)
		}
		drawRect(dst, r, s.Border, false)
	} else if on || highlighted {
		// Narrow active cell: right-edge separator only (1px vs 2px)
		drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), s.Border, true)
	}
	// Narrow inactive: no border — row stripe background provides separation
}

// DrumRowStyle is reserved for future customisation of entire rows.
type DrumRowStyle struct{}

// syncToggleVisual updates a toggle button's chrome, icon, and icon-tint to
// match an external boolean state, in one place, the same way for every
// toggle. It supports both common patterns:
//
//   - play/stop pattern: pass the same style for on and off; only the icon
//     glyph and icon color flip with state. Mirrors SetPlaying's behavior on
//     the play button.
//
//   - row-control pattern: pass different styles for on and off (e.g.
//     MuteActiveStyle vs InstButtonStyle); the fill/border flip while the
//     icon stays the same. Pass IconID("") for both icon args to leave the
//     icon untouched, and nil for both icon-color args to leave the tint
//     untouched.
//
// nil button is a no-op.
func syncToggleVisual(
	btn *Button,
	on bool,
	onStyle, offStyle ButtonVisual,
	onIcon, offIcon IconID,
	onIconColor, offIconColor color.Color,
) {
	if btn == nil {
		return
	}
	if on {
		btn.Style = onStyle
	} else {
		btn.Style = offStyle
	}
	if onIcon != "" && offIcon != "" {
		if on {
			btn.Icon = string(onIcon)
		} else {
			btn.Icon = string(offIcon)
		}
	}
	if onIconColor != nil && offIconColor != nil {
		if on {
			btn.IconColor = onIconColor
		} else {
			btn.IconColor = offIconColor
		}
	}
	btn.pressed = on
}
