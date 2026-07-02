package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// KnobScale describes the param domain a Knob represents so the widget can
// snap discrete params to detents, accumulate endless drags in real units,
// and feed the step badge / numeric editor. Set by the owning panel; the
// zero value leaves the knob in legacy continuous-absolute mode.
type KnobScale struct {
	Min, Max float64
	Unit     string   // "", "ms", "Hz", "dB", "%", "st"
	Enum     []string // non-nil => discrete
	Step     float64  // >0 => discrete detents at Min, Min+Step, ... Max
}

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

	// Bipolar, when true, renders the value arc filling outward from the
	// param's neutral point (ZeroFrac) instead of from the track start, so a
	// neutral value (+0 dB gain, 0 cents detune, +0 st pitch) shows no fill
	// and the arc grows on either side of neutral. Set by the synth/sampler
	// panels for params whose range straddles zero; the drag math is
	// unchanged (still a 0..1 normalized value).
	Bipolar bool
	// ZeroFrac is the normalized [0,1] position of the param's zero, used
	// only when Bipolar is true. Symmetric ranges (±X) give 0.5; asymmetric
	// ranges (gain -24..+6 dB) give a non-centre value (0.8) so the fill
	// pivots on the true neutral, not the geometric middle.
	ZeroFrac float64

	// Scale describes the param domain (see KnobScale). Discrete/Endless are
	// derived from it by the owning panel and set explicitly.
	Scale KnobScale
	// Discrete makes drag/wheel snap to evenly spaced detents (clicky). Set
	// when Scale.Enum != nil || Scale.Step > 0.
	Discrete bool
	// Endless makes drag accumulate incrementally in real units (StepMul per
	// knobEndlessPxPerNotch px) instead of mapping the rect to 0..1, so the
	// turn never hits a travel wall. Mutually exclusive with Discrete.
	Endless bool
	// StepMul is the real-unit change applied per knobEndlessPxPerNotch px of
	// endless drag and per wheel notch. Set from the step badge.
	StepMul float64
	// endlessLastX is the previous drag X for incremental accumulation.
	endlessLastX int
	// valueWheelAccum counts signed wheel notches toward the next value step so
	// a two-finger horizontal trackpad drag moves the value slowly and clicky
	// (one step per knobValueWheelNotchesPerStep notches) instead of racing.
	// See StepValueByWheel.
	valueWheelAccum int
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

	// knobEndlessPxPerNotch is the horizontal pixels that equal one StepMul of
	// value change in endless mode. 8 px = one step gives fine finger control.
	knobEndlessPxPerNotch = 8
)

// knobValueArcSpan returns the [from,to] angular span (radians) the filled
// value arc should occupy for a normalized value in [0,1], plus whether there
// is anything to draw. Pure function of value + polarity so it is unit-tested
// directly (TestKnobValueArcSpan*).
//
//   - Unipolar: the arc runs from the track start to the value angle; value 0
//     draws nothing.
//   - Bipolar: the arc runs between the neutral angle (zeroFrac of the sweep)
//     and the value angle (neutral→value above zero, value→neutral below), so
//     value == zeroFrac draws nothing and the fill grows on either side of the
//     param's true zero.
func knobValueArcSpan(value, zeroFrac float64, bipolar bool) (from, to float32, draw bool) {
	start := float32(knobArcStartDeg * math.Pi / 180)
	valRad := start + float32(value*knobArcSweepDeg*math.Pi/180)
	if !bipolar {
		if valRad > start {
			return start, valRad, true
		}
		return 0, 0, false
	}
	zeroRad := start + float32(zeroFrac*knobArcSweepDeg*math.Pi/180)
	switch {
	case valRad > zeroRad:
		return zeroRad, valRad, true
	case valRad < zeroRad:
		return valRad, zeroRad, true
	default:
		return 0, 0, false
	}
}

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
			k.endlessLastX = mx
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

// HandleWheel adjusts the value by `steps` wheel notches. For discrete knobs,
// each notch advances exactly one detent index. For endless knobs, each notch
// applies ±StepMul real units (clamped to [Min,Max]). For legacy continuous
// knobs the original knobWheelStep behaviour is preserved. Returns
// InputConsumed when the value changed, InputIgnored when the cursor is
// outside the knob's rect or the value is already at its limit.
func (k *Knob) HandleWheel(mx, my, steps int) InputResult {
	if !image.Pt(mx, my).In(k.r) {
		return InputIgnored
	}
	if k.Discrete {
		n := k.detentCount()
		if n >= 2 {
			idx := int(math.Round(k.Value*float64(n-1))) + steps
			if idx < 0 {
				idx = 0
			} else if idx > n-1 {
				idx = n - 1
			}
			nv := float64(idx) / float64(n-1)
			if nv == k.Value {
				return InputIgnored
			}
			k.Value = nv
			return InputConsumed
		}
	}
	if k.Endless {
		span := k.Scale.Max - k.Scale.Min
		if span > 0 && k.StepMul > 0 {
			real := clampF64(k.Scale.Min+k.Value*span+float64(steps)*k.StepMul, k.Scale.Min, k.Scale.Max)
			nv := (real - k.Scale.Min) / span
			if nv == k.Value {
				return InputIgnored
			}
			k.Value = nv
			return InputConsumed
		}
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

// knobValueWheelNotchesPerStep is how many two-finger/wheel notches advance the
// knob VALUE by one step. >1 makes the wheel slow and human-paced; raise it to
// slow further. (The per-step AMOUNT is StepMul, which the user controls via
// the resolution badge — so resolution × cadence both shape the feel.)
const knobValueWheelNotchesPerStep = 4

// StepValueByWheel feeds a raw wheel/two-finger delta toward a value change.
// Each call counts as a SINGLE notch regardless of magnitude (so a fast or
// high-resolution trackpad can't race the value), and the value only advances
// by ONE step once knobValueWheelNotchesPerStep notches accumulate in one
// direction — giving a deliberate, easy-to-control feel. One step = ±StepMul
// for endless knobs (honoring the user's chosen resolution) or ±1 detent for
// discrete knobs. A reversal discards leftover travel. Returns true if the
// value changed. Unlike HandleWheel it does NOT range-check the cursor, so it
// works whether the gesture is over the dial, the caption, or the readout.
func (k *Knob) StepValueByWheel(steps int) bool {
	if steps == 0 {
		return false
	}
	dir := 1
	if steps < 0 {
		dir = -1
	}
	if k.valueWheelAccum != 0 && (k.valueWheelAccum > 0) != (dir > 0) {
		k.valueWheelAccum = 0
	}
	k.valueWheelAccum += dir
	if k.valueWheelAccum > -knobValueWheelNotchesPerStep && k.valueWheelAccum < knobValueWheelNotchesPerStep {
		return false
	}
	k.valueWheelAccum = 0
	return k.applyValueStep(dir)
}

// StepValueDirect advances the value by exactly ONE step in dir (+1/-1) with
// no internal accumulator — one detent for discrete knobs, one StepMul for
// endless knobs. Use this when the caller already manages its own debounce
// accumulator (e.g. MobileWheelPopup's discAcc), so the extra per-notch
// threshold of StepValueByWheel is not desirable.
func (k *Knob) StepValueDirect(dir int) bool { return k.applyValueStep(dir) }

// applyValueStep moves the value by exactly one step in dir (+1/-1) — one
// detent for discrete knobs, one StepMul for endless knobs — with no cursor
// range check. Returns true if the value changed.
func (k *Knob) applyValueStep(dir int) bool {
	if k.Discrete {
		n := k.detentCount()
		if n < 2 {
			return false
		}
		idx := int(math.Round(k.Value*float64(n-1))) + dir
		if idx < 0 {
			idx = 0
		} else if idx > n-1 {
			idx = n - 1
		}
		nv := float64(idx) / float64(n-1)
		if nv == k.Value {
			return false
		}
		k.Value = nv
		return true
	}
	span := k.Scale.Max - k.Scale.Min
	if span <= 0 {
		return false
	}
	step := k.StepMul
	if step <= 0 {
		step = span / 200
	}
	real := clampF64(k.Scale.Min+k.Value*span+float64(dir)*step, k.Scale.Min, k.Scale.Max)
	nv := (real - k.Scale.Min) / span
	if nv == k.Value {
		return false
	}
	k.Value = nv
	return true
}

// detentCount returns the number of discrete positions, or 0 if continuous.
func (k *Knob) detentCount() int {
	if len(k.Scale.Enum) > 0 {
		return len(k.Scale.Enum)
	}
	if k.Scale.Step > 0 && k.Scale.Max > k.Scale.Min {
		return int(math.Round((k.Scale.Max-k.Scale.Min)/k.Scale.Step)) + 1
	}
	return 0
}

// snapToDetent rounds a normalized [0,1] value to the nearest detent.
func (k *Knob) snapToDetent(v float64) float64 {
	n := k.detentCount()
	if n < 2 {
		return v
	}
	idx := int(math.Round(v * float64(n-1)))
	if idx < 0 {
		idx = 0
	} else if idx > n-1 {
		idx = n - 1
	}
	return float64(idx) / float64(n-1)
}

func (k *Knob) updateFromDrag(mx, my int) {
	if k.Endless {
		k.updateEndless(mx)
		return
	}
	pixels := k.dragPixels()
	// Horizontal-only: right = increase, left = decrease. Vertical motion is
	// deliberately ignored so dragging a knob never fights the panel's up/down
	// scroll — the two gestures are on orthogonal axes. (my is unused.)
	dxPixels := mx - k.pressX // right = positive
	v := k.pressVal + float64(dxPixels)/float64(pixels)
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if k.Discrete {
		v = k.snapToDetent(v)
	}
	k.Value = v
}

// updateEndless accumulates an incremental drag in real units so the knob has
// no travel limit; the arc still fills proportionally to value-in-range.
func (k *Knob) updateEndless(mx int) {
	span := k.Scale.Max - k.Scale.Min
	if span <= 0 {
		return
	}
	step := k.StepMul
	if step <= 0 {
		step = span / 200
	}
	d := mx - k.endlessLastX
	k.endlessLastX = mx
	real := k.Scale.Min + k.Value*span
	real = clampF64(real+(float64(d)/float64(knobEndlessPxPerNotch))*step, k.Scale.Min, k.Scale.Max)
	k.Value = (real - k.Scale.Min) / span
}

// NudgeEndless accumulates an endless value change by deltaPx pixels of motion
// (positive = increase). Delta-based sibling of updateEndless: the caller owns
// the per-frame delta, so this carries no latched state and is safe to drive
// from the mobile wheel popup's vertical drag. Clamps at Scale.Min/Max.
func (k *Knob) NudgeEndless(deltaPx int) {
	span := k.Scale.Max - k.Scale.Min
	if span <= 0 {
		return
	}
	step := k.StepMul
	if step <= 0 {
		step = span / 200
	}
	real := k.Scale.Min + k.Value*span
	real = clampF64(real+(float64(deltaPx)/float64(knobEndlessPxPerNotch))*step, k.Scale.Min, k.Scale.Max)
	k.Value = (real - k.Scale.Min) / span
}

// Draw renders the knob — background ring, value arc, indicator line, centre
// dot. The caption (`name  value unit`) is drawn separately by the calling
// panel because the unit text is param-specific.
// knurledCapKey is just the diameter — the static cap (knurled rim + dome +
// side wall + contact shadow) is value/travel-invariant, so it is rasterized
// once per diameter and blitted each frame; only the live value arc + pointer
// draw on top. This keeps per-frame knob cost a single blit + the existing two
// drawArc calls — critical on the synth tab's many small knobs under the WASM
// single audio thread.
var knurledCapCache = map[int]*ebiten.Image{}

const knurledRidges = 18

// knurledCapSprite returns the cached static knob-cap sprite of the given
// diameter (square). Rasterized once, blitted thereafter.
func knurledCapSprite(diameter int) *ebiten.Image {
	if diameter < 2 {
		diameter = 2
	}
	if spr, ok := knurledCapCache[diameter]; ok {
		return spr
	}
	spr := ebiten.NewImage(diameter, diameter)
	cx, cy := float64(diameter)/2, float64(diameter)/2
	rad := float32(diameter) / 2
	wall := TokenSurface1()
	body := TokenSurface2()
	dome := TokenSurface3()
	// Contact shadow (offset 1px down) + side wall.
	vector.DrawFilledCircle(spr, float32(cx), float32(cy)+1, rad, WithAlphaFromColor(color.Black, 64), true)
	vector.DrawFilledCircle(spr, float32(cx), float32(cy), rad, wall, true)
	// Knurled rim: alternating light/dark radial ridges.
	for i := 0; i < knurledRidges; i++ {
		a := float64(i) / float64(knurledRidges) * 2 * math.Pi
		var col color.Color = dome
		if i%2 == 0 {
			col = adjustColor(dome, 18)
		}
		ix := cx + math.Cos(a)*float64(rad)*0.74
		iy := cy + math.Sin(a)*float64(rad)*0.74
		ox := cx + math.Cos(a)*float64(rad)*0.96
		oy := cy + math.Sin(a)*float64(rad)*0.96
		vector.StrokeLine(spr, float32(ix), float32(iy), float32(ox), float32(oy), 2, col, true)
	}
	// Matte center dome — concentric body/dome rings, no specular highlight
	// (retro-analogue restyle 2026-06-17).
	vector.DrawFilledCircle(spr, float32(cx), float32(cy), rad*0.72, body, true)
	vector.DrawFilledCircle(spr, float32(cx), float32(cy), rad*0.5, dome, true)
	knurledCapCache[diameter] = spr
	return spr
}

func (k *Knob) Draw(dst *ebiten.Image) {
	if k.r.Empty() {
		return
	}
	cx, cy, radius := k.geom()
	if radius < 4 {
		return
	}

	// Static volumetric cap (cached), blitted centered under the live arc.
	d := int(radius * 2)
	if spr := knurledCapSprite(d); spr != nil {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(cx-float64(d)/2, cy-float64(d)/2)
		dst.DrawImage(spr, &op)
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

	// Value arc. Unipolar: grows from the track start. Bipolar: grows from
	// the 12-o'clock centre outward (see knobValueArcSpan).
	if from, to, draw := knobValueArcSpan(k.Value, k.ZeroFrac, k.Bipolar); draw {
		drawArc(dst, cx, cy, radius, from, to, strokeWidth, fillCol)
	}

	// Detent ticks for discrete knobs: short radial marks at each stop.
	if k.Discrete {
		k.drawDetents(dst, cx, cy, float64(radius), strokeWidth, TokenTextSecondary())
	}

	// Indicator points at the current value angle regardless of polarity.
	valRad := startRad + float32(k.Value*knobArcSweepDeg*math.Pi/180)

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

// detentTickDrawer is the per-detent draw hook, swappable in tests to count
// detents without reading pixels.
var detentTickDrawer = func(angleRad float64) {}

func swapDetentTickDrawerForTest(fn func(float64)) func() {
	prev := detentTickDrawer
	detentTickDrawer = fn
	return func() { detentTickDrawer = prev }
}

// drawDetents strokes a short radial tick at each detent position around the
// arc, so a discrete knob visibly reads as N fixed stops.
func (k *Knob) drawDetents(dst *ebiten.Image, cx, cy, radius float64, strokeWidth float32, col color.Color) {
	n := k.detentCount()
	if n < 2 {
		return
	}
	start := knobArcStartDeg * math.Pi / 180
	sweep := knobArcSweepDeg * math.Pi / 180
	innerR := radius - float64(strokeWidth)*1.6
	outerR := radius + float64(strokeWidth)*0.2
	if innerR < 1 {
		innerR = 1
	}
	for i := 0; i < n; i++ {
		a := start + sweep*float64(i)/float64(n-1)
		detentTickDrawer(a)
		ix := cx + math.Cos(a)*innerR
		iy := cy + math.Sin(a)*innerR
		ox := cx + math.Cos(a)*outerR
		oy := cy + math.Sin(a)*outerR
		vector.StrokeLine(dst, float32(ix), float32(iy), float32(ox), float32(oy), 1.5, col, true)
	}
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
