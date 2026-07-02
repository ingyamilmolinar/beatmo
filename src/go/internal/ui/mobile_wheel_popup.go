// src/go/internal/ui/mobile_wheel_popup.go
package ui

import (
	"image"
	"image/color"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// wheelDragKind disambiguates which sub-control the active drag belongs to.
type wheelDragKind int

const (
	wheelDragNone wheelDragKind = iota
	wheelDragValue
	wheelDragRes
)

// WheelBinding wires a MobileWheelPopup to the single shared knob value-model.
// The popup stores NO value of its own — it reads/writes Knob and Badge.
type WheelBinding struct {
	Knob         *Knob
	Badge        *KnobStepBadge // nil for discrete/enum params (no resolution strip)
	Def          audio.ParamDef
	Discrete     bool // enum or Step>0: step detents instead of endless accumulation
	OnChange     func()
	OnCommit     func()
	OnResolution func()
	OpenEditor   func()
	Title        func() string
	Accent       func() color.Color
}

// MobileWheelPopup is the mobile vertical scroll-wheel view over a single knob.
type MobileWheelPopup struct {
	binding  WheelBinding
	open     bool
	rect     image.Rectangle
	anchor   image.Rectangle
	titleRect image.Rectangle // reserved title band at the panel top
	resRect  image.Rectangle // resolution strip sub-rect (set in layout)
	valRect  image.Rectangle // barrel sub-rect
	centerH  image.Rectangle // center value box (tap → numeric entry)
	dragging   bool
	dragKind   wheelDragKind
	lastY      int
	valNotcher   stepNotcher // clicky cadence for value drag (one step per notch)
	resNotcher   stepNotcher // clicky cadence for resolution-strip drag
	wheelNotcher stepNotcher // paces mouse-wheel / two-finger scroll (events per step)
}

// wheelStepEventsPerNotch is how many discrete scroll events advance the value by
// one step. Scroll magnitude is ignored (a two-finger trackpad flick reports a
// large delta per event); pacing by event count keeps wheel scrolling slow and
// controlled, matching the clicky drag cadence.
const wheelStepEventsPerNotch = 3

func NewMobileWheelPopup() *MobileWheelPopup { return &MobileWheelPopup{} }

func (w *MobileWheelPopup) Open(b WheelBinding, anchor, bounds image.Rectangle, headerH int) {
	w.binding = b
	w.anchor = anchor
	dvals := Profile().DensityValues()
	pw, ph := dvals.MobileWheelW, dvals.MobileWheelH
	w.rect = AnchorPopupRect(bounds, anchor, pw, ph, PopupBelow)
	w.layout()
	w.open = true
	w.dragging = false
	w.dragKind = wheelDragNone
	w.valNotcher = newStepNotcher(w.tickGap())
	w.resNotcher = newStepNotcher(w.tickGap())
	w.wheelNotcher = newStepNotcher(wheelStepEventsPerNotch)
}

// layout splits the panel into a barrel column, an optional resolution strip,
// and a center value box. Discrete params hide the strip.
func (w *MobileWheelPopup) layout() {
	dvals := Profile().DensityValues()
	inner := w.rect.Inset(SpaceSM)

	// Reserve a title band at the top so the barrel never collides with the title.
	titleH := StyledTextHeight(RoleBody) + SpaceSM*2
	w.titleRect = image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, inner.Min.Y+titleH)
	body := image.Rect(inner.Min.X, inner.Min.Y+titleH, inner.Max.X, inner.Max.Y)

	resW := 0
	if w.binding.Badge != nil && !w.binding.Discrete {
		resW = dvals.MobileWheelResStripW
	}
	if resW > 0 {
		// Barrel column on the left; resolution strip on the right with a gap.
		w.valRect = image.Rect(body.Min.X, body.Min.Y, body.Max.X-resW-SpaceSM, body.Max.Y)
		w.resRect = image.Rect(body.Max.X-resW, body.Min.Y, body.Max.X, body.Max.Y)
	} else {
		w.valRect = body
		w.resRect = image.Rectangle{}
	}
	// center value box: a band centered vertically in the barrel column.
	pillH := dvals.MobileWheelPillH
	cy := (w.valRect.Min.Y + w.valRect.Max.Y) / 2
	w.centerH = image.Rect(w.valRect.Min.X, cy-pillH/2, w.valRect.Max.X, cy+pillH/2)
}

func (w *MobileWheelPopup) Close()                  { w.open = false; w.dragging = false; w.dragKind = wheelDragNone }
func (w *MobileWheelPopup) IsOpen() bool            { return w.open }
func (w *MobileWheelPopup) Rect() image.Rectangle   { return w.rect }
func (w *MobileWheelPopup) Anchor() image.Rectangle { return w.anchor }
func (w *MobileWheelPopup) IsDragging() bool { return w.dragging && w.dragKind == wheelDragValue }

// IsDraggingAny reports whether ANY drag (value OR resolution strip) is active.
// The portal hit handler uses this to claim capture so move events keep flowing
// to the popup; the narrower IsDragging() stays value-only for row-scroll isolation.
func (w *MobileWheelPopup) IsDraggingAny() bool { return w.dragging }

// HandleWheel applies a mouse-wheel / trackpad two-finger scroll. Scroll
// MAGNITUDE is deliberately ignored (a two-finger flick reports a large delta
// per event, which used to fly the value); instead each event contributes one
// unit of direction to a notcher, so it takes wheelStepEventsPerNotch events to
// advance one step — slow and controlled, matching the clicky drag. Over the
// resolution strip it shifts the rung at the same cadence. Returns true if it
// consumed the event; each emitted step commits so wheel edits land in undo.
func (w *MobileWheelPopup) HandleWheel(mx, my, steps int) bool {
	if !w.open || steps == 0 {
		return false
	}
	dir := 1
	if steps < 0 {
		dir = -1
	}
	n := w.wheelNotcher.add(dir) // paced: magnitude-independent, events-per-step
	overStrip := !w.resRect.Empty() && image.Pt(mx, my).In(w.resRect) && w.binding.Badge != nil
	if n == 0 {
		return true // consumed, but not enough scroll has accumulated for a step yet
	}
	if overStrip {
		changed := false
		for i := 0; i < n; i++ {
			changed = w.binding.Badge.AdjustResolution(1) || changed
		}
		for i := 0; i > n; i-- {
			changed = w.binding.Badge.AdjustResolution(-1) || changed
		}
		if changed {
			if w.binding.Knob != nil {
				w.binding.Knob.StepMul = w.binding.Badge.Step()
			}
			if w.binding.OnResolution != nil {
				w.binding.OnResolution()
			}
		}
		return true
	}
	k := w.binding.Knob
	if k == nil {
		return false
	}
	if w.binding.Discrete {
		applyKnobSteps(k, n)
	} else {
		k.NudgeEndless(n * knobEndlessPxPerNotch)
	}
	if w.binding.OnChange != nil {
		w.binding.OnChange()
	}
	if w.binding.OnCommit != nil {
		w.binding.OnCommit()
	}
	return true
}

// HandleInput routes a pointer event. Returns true if the popup consumed it.
func (w *MobileWheelPopup) HandleInput(mx, my int, pressed bool) bool {
	if !w.open {
		return false
	}
	if !pressed {
		if w.dragging && w.dragKind == wheelDragValue && w.binding.OnCommit != nil {
			w.binding.OnCommit()
		}
		w.dragging = false
		w.dragKind = wheelDragNone
		return true
	}
	pt := image.Pt(mx, my)
	if !w.dragging {
		// Press: decide which sub-control this gesture owns.
		switch {
		case !w.binding.Discrete && pt.In(w.centerH) && w.binding.OpenEditor != nil:
			w.binding.OpenEditor()
			return true
		case !w.resRect.Empty() && pt.In(w.resRect):
			w.dragging = true
			w.dragKind = wheelDragRes
			w.lastY = my
			w.resNotcher.setPxPerNotch(w.tickGap())
			w.resNotcher.reset()
		default:
			w.dragging = true
			w.dragKind = wheelDragValue
			w.lastY = my
			w.valNotcher.setPxPerNotch(w.tickGap())
			w.valNotcher.reset()
		}
		return true
	}
	// Continue drag.
	switch w.dragKind {
	case wheelDragValue:
		delta := w.lastY - my // finger up (y decreases) => positive => increase
		w.lastY = my
		if w.applyValueDelta(delta) && w.binding.OnChange != nil {
			w.binding.OnChange() // only when a detent actually clicked over
		}
	case wheelDragRes:
		w.applyResDelta(w.lastY - my)
		w.lastY = my
	}
	return true
}

// applyValueDelta feeds a vertical delta through the clicky cadence notcher and
// advances the value one detent per notch — the same step-by-step feel as a
// discrete knob, so a tiny drag no longer flies through every value. Continuous
// params move exactly one StepMul per notch (snapping to the step grid); enum
// params advance one item per notch (list-scroll: drag down = next). Returns
// true if at least one detent clicked over.
func (w *MobileWheelPopup) applyValueDelta(delta int) bool {
	k := w.binding.Knob
	if k == nil {
		return false
	}
	if w.binding.Discrete {
		// Enum list-scroll: drag down (delta<0) advances the index, so invert.
		steps := w.valNotcher.add(-delta)
		if steps == 0 {
			return false
		}
		applyKnobSteps(k, steps)
		return true
	}
	steps := w.valNotcher.add(delta)
	if steps == 0 {
		return false
	}
	// One notch == one StepMul: NudgeEndless(knobEndlessPxPerNotch) is exactly
	// one StepMul, so steps*knobEndlessPxPerNotch advances steps grid-steps.
	k.NudgeEndless(steps * knobEndlessPxPerNotch)
	return true
}

// applyResDelta drags the resolution strip across the KnobStepBadge rungs and
// keeps the knob's StepMul in sync. One rung shift per tick-gap of travel.
func (w *MobileWheelPopup) applyResDelta(delta int) {
	b := w.binding.Badge
	if b == nil {
		return
	}
	// Same clicky cadence as the value drag: one rung per notch of travel.
	steps := w.resNotcher.add(delta)
	changed := false
	for i := 0; i < steps; i++ { // drag up = finer
		if b.AdjustResolution(1) {
			changed = true
		}
	}
	for i := 0; i > steps; i-- { // drag down = coarser
		if b.AdjustResolution(-1) {
			changed = true
		}
	}
	if changed {
		if w.binding.Knob != nil {
			w.binding.Knob.StepMul = b.Step()
		}
		if w.binding.OnResolution != nil {
			w.binding.OnResolution()
		}
	}
}
