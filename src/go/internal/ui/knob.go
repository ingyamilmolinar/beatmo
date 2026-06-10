package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Knob is a rotary control with a 0..1 value, used by the Synth tab. The
// public API mirrors Slider (SetRect / Rect / Value / HandleInputResult /
// Capturing / Draw) so the existing hit-area + propagate machinery in
// synth_panel_zone.go can drive either widget interchangeably.
//
// Interaction model: press anywhere inside the knob's rect → drag
// captures. Motion drives the value HORIZONTALLY ONLY: right = increase,
// left = decrease. Vertical motion is intentionally ignored so a knob
// drag never competes with the panel's up/down scroll — left/right tweaks
// the value, up/down is free for scrolling (the synth panel hands a
// vertical drag that starts on a knob to its section scroll; see
// synthKnobHitAdapter). Default sensitivity is one full 0..1 sweep per
// knobDragPixelsCoarse pixels of horizontal travel; FineDrag (used by the
// Synth-tab Shift-drag modifier) uses knobDragPixelsFine. The press point
// + initial value are latched so a single uninterrupted drag never
// accumulates drift from frame jitter.
//
// Visual model: a circular arc occupies the upper portion of the rect with a
// notch at the bottom (~270° sweep, mirroring Drum Bus / Vital). The accent
// arc fills clockwise from the 7-o'clock position to the value-proportional
// angle; an indicator line points from the centre to the value position.
// The lower portion of the rect is reserved for the caption that
// synth_panel_sections.go draws on top of the widget.
type Knob struct {
	r        image.Rectangle
	Value    float64
	dragging bool
	pressX   int
	pressY   int
	pressVal float64

	// FineDrag, when true, increases the pixels-per-sweep so each pixel of
	// movement changes the value by a smaller amount. The Synth tab
	// toggles this when the Shift modifier is held during a drag.
	FineDrag bool
}

const (
	// knobDragPixelsCoarse is the horizontal-pixel distance required to sweep
	// the value from 0 to 1 (or 1 to 0) in normal drag mode. Chosen so a
	// typical drag (~120 px) covers the full range without requiring the
	// user to leave the panel.
	knobDragPixelsCoarse = 120
	// knobDragPixelsFine is the equivalent in fine mode (Shift-drag).
	// Three-times slower so a full mouse-wheel-area drag covers only a third
	// of the range — enough resolution for cents / single-percent edits.
	knobDragPixelsFine = 360

	// knobArcSweepDeg is the total angular extent of the arc in degrees.
	// 270° leaves a 90° notch at the bottom (45° on each side of straight
	// down), mirroring conventional DAW rotaries.
	knobArcSweepDeg = 270.0
	// knobArcStartDeg is the angle (in degrees, measured clockwise from
	// the +X axis using screen coords) at which the empty arc begins. 135°
	// places the start at the lower-left, ending at lower-right.
	knobArcStartDeg = 135.0

	// knobWheelStep is the value delta per wheel notch in coarse mode
	// (2.5 % of the 0..1 range). FineDrag halves this.
	knobWheelStep     = 0.025
	knobWheelStepFine = 0.005
)

// NewKnob returns a Knob initialised to v (clamped to [0,1]).
func NewKnob(v float64) *Knob {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return &Knob{Value: v}
}

// SetRect sets the bounding rectangle the knob is drawn inside.
func (k *Knob) SetRect(r image.Rectangle) { k.r = r }

// Rect returns the current bounding rectangle.
func (k *Knob) Rect() image.Rectangle { return k.r }

// Capturing reports whether a drag is in progress.
func (k *Knob) Capturing() bool { return k.dragging }

// HandleInputResult dispatches a pointer event. Returns InputCaptured during
// an active drag, InputConsumed on release after a drag, and InputIgnored
// when the knob is not involved.
//
// Release semantics: release is a *commit* of the drag-end value, not a
// re-evaluation. OnDrag has already published the latest value every captured
// frame, so Value already reflects the drag-end position. Re-running
// updateFromDrag on release coords would only add noise on desktop — and on
// mobile it is the failure mode: the dispatcher can hand stale coords (e.g.,
// (0,0) on the touch-end frame, see the corresponding hold in
// updateTouchOverride) which would push Value to an extreme. This matches
// Slider/SliderPopup, which also treat release as a flag-clear only.
func (k *Knob) HandleInputResult(mx, my int, pressed bool) InputResult {
	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		k.dragging = false
		return InputIgnored
	}
	if pressed {
		if !k.dragging {
			if !image.Pt(mx, my).In(k.r) {
				return InputIgnored
			}
			k.dragging = true
			k.pressX = mx
			k.pressY = my
			k.pressVal = k.Value
			return InputCaptured
		}
		k.updateFromDrag(mx, my)
		return InputCaptured
	}
	if k.dragging {
		// Release: commit the existing Value; do NOT recompute from (mx, my).
		k.dragging = false
		return InputConsumed
	}
	return InputIgnored
}

// dragPixels is the pixels-per-full-sweep distance for the current drag mode,
// scaled by knob radius so larger knobs (mobile/Spacious density:
// KnobIdeal≈88, radius≈44) get a proportionally longer sweep — roughly 2x
// finer than the fixed-constant default. The minimum is the original constant
// so small (Compact-density) knobs feel identical to before. The multiplier
// of 4 was chosen so radius≈44 yields ≈176 px sweep — about 2x the original
// 88 px diameter, giving room for nuanced finger travel without forcing the
// user to leave the panel.
func (k *Knob) dragPixels() int {
	base := knobDragPixelsCoarse
	if k.FineDrag {
		base = knobDragPixelsFine
	}
	_, _, radius := k.geom()
	radiusScaled := int(radius * 4)
	if radiusScaled > base {
		return radiusScaled
	}
	return base
}

// HandleWheel adjusts the value by `steps` wheel notches. Each notch
// produces `knobWheelStep` of value change (0.025 = 2.5 %, ≈ one notch
// per 1.6 % of the range). Returns InputConsumed when the value changed,
// InputIgnored when the click is outside the knob's rect.
func (k *Knob) HandleWheel(mx, my, steps int) InputResult {
	if !image.Pt(mx, my).In(k.r) {
		return InputIgnored
	}
	step := knobWheelStep
	if k.FineDrag {
		step = knobWheelStepFine
	}
	v := k.Value + float64(steps)*step
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if v == k.Value {
		return InputIgnored
	}
	k.Value = v
	return InputConsumed
}

func (k *Knob) updateFromDrag(mx, my int) {
	pixels := k.dragPixels()
	// Horizontal-only: right = increase, left = decrease. Vertical motion is
	// deliberately ignored so dragging a knob never fights the panel's up/down
	// scroll — the two gestures are on orthogonal axes. (my is unused.)
	dxPixels := mx - k.pressX // right = positive
	delta := float64(dxPixels) / float64(pixels)
	v := k.pressVal + delta
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	k.Value = v
}

// Draw renders the knob — background ring, value arc, indicator line, centre
// dot. The caption (`name  value unit`) is drawn separately by the calling
// panel because the unit text is param-specific.
func (k *Knob) Draw(dst *ebiten.Image) {
	if k.r.Empty() {
		return
	}
	cx, cy, radius := k.geom()
	if radius < 4 {
		return
	}

	trackCol := TokenSurface3()
	fillCol := TokenAccentDim()
	if k.dragging {
		fillCol = TokenAccent()
	}
	indicatorCol := TokenAccent()

	startRad := float32(knobArcStartDeg * math.Pi / 180)
	endRad := float32((knobArcStartDeg + knobArcSweepDeg) * math.Pi / 180)
	strokeWidth := float32(radius) * 0.18
	if strokeWidth < 2 {
		strokeWidth = 2
	}

	// Background ring (full sweep, muted).
	drawArc(dst, cx, cy, radius, startRad, endRad, strokeWidth, trackCol)

	// Value arc (from start through value-proportional angle).
	valRad := startRad + float32(k.Value*knobArcSweepDeg*math.Pi/180)
	if valRad > startRad {
		drawArc(dst, cx, cy, radius, startRad, valRad, strokeWidth, fillCol)
	}

	// Indicator: short line from inner edge of arc to the centre disk.
	innerR := radius - strokeWidth*1.3
	outerR := radius - strokeWidth*0.4
	if innerR < 1 {
		innerR = 1
	}
	ix := cx + math.Cos(float64(valRad))*float64(innerR)
	iy := cy + math.Sin(float64(valRad))*float64(innerR)
	ox := cx + math.Cos(float64(valRad))*float64(outerR)
	oy := cy + math.Sin(float64(valRad))*float64(outerR)
	vector.StrokeLine(dst, float32(ix), float32(iy), float32(ox), float32(oy), strokeWidth, indicatorCol, true)

	// Centre disk for visual anchor.
	dotR := strokeWidth * 0.9
	if dotR < 1 {
		dotR = 1
	}
	vector.DrawFilledCircle(dst, float32(cx), float32(cy), dotR, trackCol, true)
}

// geom resolves the centre point + radius of the knob given its rect. The
// knob is centred horizontally and biased toward the top of the rect to
// leave space below for the caption.
func (k *Knob) geom() (cx, cy float64, radius float32) {
	w := k.r.Dx()
	h := k.r.Dy()
	side := w
	if h < side {
		side = h
	}
	// Reserve ~25% of the rect height for the caption when the rect is
	// tall enough to accommodate one; for square rects the knob occupies
	// the full short dimension.
	if h >= w*5/4 {
		side = w
	}
	radius = float32(side) / 2.0
	cx = float64(k.r.Min.X) + float64(w)/2.0
	cy = float64(k.r.Min.Y) + float64(side)/2.0
	return
}

// drawArcVS / drawArcIS are package-level scratch buffers reused across
// every drawArc invocation. Ebiten Update + Draw run on a single UI
// goroutine, so no synchronisation is needed. Pre-fix, each drawArc
// allocated fresh []ebiten.Vertex and []uint16 slices — at ~30 knobs ×
// 2 drawArc calls/knob × 30 fps on the synth tab, that contributed
// ~0.7 MB/s of allocation pressure that Go's WASM single-threaded GC
// could not promptly reclaim. The reuse pattern drops the vertex/index
// portion to ~0 MB/s steady state while preserving the
// AppendVerticesAndIndicesForStroke contract (caller resets length to
// 0 before each call; the function appends into the provided slices).
// vector.Path is not pooled because ebiten/v2 doesn't expose a public
// Reset method — it's allocated fresh each call but its internal slices
// are smaller than the vs/is buffers.
var (
	drawArcVS []ebiten.Vertex
	drawArcIS []uint16
)

// drawArc strokes a circular arc from startRad to endRad (clockwise in
// screen coords, i.e. +Y is down) using the given stroke width.
func drawArc(dst *ebiten.Image, cx, cy float64, radius, startRad, endRad, width float32, col color.Color) {
	if endRad <= startRad {
		return
	}
	const segmentsPerSweep = 96
	totalSweep := float64(endRad - startRad)
	segments := int(math.Ceil(totalSweep / (2 * math.Pi) * segmentsPerSweep))
	if segments < 6 {
		segments = 6
	}
	var p vector.Path
	for i := 0; i <= segments; i++ {
		a := float64(startRad) + totalSweep*float64(i)/float64(segments)
		x := cx + math.Cos(a)*float64(radius)
		y := cy + math.Sin(a)*float64(radius)
		if i == 0 {
			p.MoveTo(float32(x), float32(y))
		} else {
			p.LineTo(float32(x), float32(y))
		}
	}
	opts := &vector.StrokeOptions{
		Width:    width,
		LineCap:  vector.LineCapRound,
		LineJoin: vector.LineJoinRound,
	}
	drawArcVS = drawArcVS[:0]
	drawArcIS = drawArcIS[:0]
	drawArcVS, drawArcIS = p.AppendVerticesAndIndicesForStroke(drawArcVS, drawArcIS, opts)
	cr, cg, cb, ca := rgbaParts(col)
	for i := range drawArcVS {
		drawArcVS[i].SrcX = 1
		drawArcVS[i].SrcY = 1
		drawArcVS[i].ColorR = cr
		drawArcVS[i].ColorG = cg
		drawArcVS[i].ColorB = cb
		drawArcVS[i].ColorA = ca
	}
	dst.DrawTriangles(drawArcVS, drawArcIS, iconWhiteSubImage(), &ebiten.DrawTrianglesOptions{
		AntiAlias: true,
		FillRule:  ebiten.FillRuleNonZero,
	})
}
