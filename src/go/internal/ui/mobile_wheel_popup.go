// src/go/internal/ui/mobile_wheel_popup.go
package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
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
	// Model, when set, is the value model the wheel scrolls (NumberModel for
	// BPM / node values / go-to). When nil the legacy Knob/Badge/Def/Discrete
	// fields are adapted through knobModel (synth, sampler, EQ).
	Model    WheelModel
	Knob     *Knob
	Badge    *KnobStepBadge // nil for discrete/enum params (no resolution strip)
	Def      audio.ParamDef
	Discrete bool // enum or Step>0: step detents instead of endless accumulation
	OnChange func()
	// OnCancel, when set, restores the bound value on Cancel (Esc) instead of
	// re-applying the model's snapshot through OnChange — for a model that
	// converts or clamps (dB ↔ linear, a floor) and so cannot round-trip the
	// opening value exactly.
	OnCancel     func()
	OnCommit     func()
	OnResolution func()
	Title        func() string
	Gloss        func() string // optional one-line PlainEnglish purpose ("bright or dull")
	Accent       func() color.Color
}

// ValueWheel is the embeddable value-wheel VIEW: a barrel of distinct value
// rows around a centre selector, plus an optional resolution ladder. It draws no
// frame and no title, so it can sit inside any card (the navigation menu's go-to
// wheel, the logs time picker) or inside MobileWheelPopup, which adds the popup
// frame, title band and the transactional Accept/Cancel. It owns no value: every
// label and step goes through the binding's WheelModel.
type ValueWheel struct {
	binding      WheelBinding
	resRect      image.Rectangle // resolution strip sub-rect (set in layoutBody)
	valRect      image.Rectangle // barrel sub-rect
	centerH      image.Rectangle // center value box
	dragging     bool
	dragKind     wheelDragKind
	lastY        int
	valNotcher   stepNotcher // clicky cadence for value drag (one step per notch)
	resNotcher   stepNotcher // clicky cadence for resolution-strip drag
	wheelNotcher stepNotcher // paces mouse-wheel / two-finger scroll (events per step)

	// Tap-vs-drag disambiguation for the barrel: a press latches its point;
	// once the pointer travels past wheelTapSlopPx the gesture is a drag. A
	// release that never moved is a CLICK — on a visible row it selects that
	// row's value directly (desktop users click values instead of scrolling).
	pressPt    image.Point
	pressMoved bool

	dirty bool // a value change happened since the host last reset it

	// accept is the host's "confirm" action, fired by a click on the centre
	// selector (MobileWheelPopup: Accept; the go-to card: Go). Nil = inert.
	accept func()

	// Plain-hover state (desktop only), refreshed once per frame via HoverAt
	// (the portalHoverer seam — portal input handlers only run on press
	// edges, so without this the ladder/center grip never show a hover cue).
	hoverRung   int  // rung index under the cursor, -1 when none
	hoverCenter bool // cursor is over the center value box

	km *knobModel // cached legacy adapter (see model)
}

// MobileWheelPopup is the value-wheel POPUP: a ValueWheel inside a floating
// panel with a title band, anchored to the control that opened it, with a
// transactional edit (live preview, one commit on Accept, restore on Cancel).
type MobileWheelPopup struct {
	ValueWheel
	open      bool
	rect      image.Rectangle
	anchor    image.Rectangle
	titleRect image.Rectangle // reserved title band at the panel top

	// Transactional edit state. The popup is a modal value editor: turning the
	// wheel previews live via OnChange, but the undo commit is DEFERRED until
	// the edit is Accepted (Enter / tap-away / navigate-away). Esc Cancels,
	// restoring initialValue. See Accept/Cancel.
	initialValue float64 // model value snapshotted at Open (for Cancel)
}

// wheelStepEventsPerNotch is how many discrete scroll events advance the value by
// one step. Scroll magnitude is ignored (a two-finger trackpad flick reports a
// large delta per event); pacing by event count keeps wheel scrolling slow and
// controlled, matching the clicky drag cadence.
const wheelStepEventsPerNotch = 3

// wheelTapSlopPx is the movement budget within which a press+release counts
// as a CLICK on a barrel row rather than a drag. Kept well below tickGap so a
// sub-slop gesture can never both fire a drag detent and a row jump.
const wheelTapSlopPx = 6

// barrelRowAt maps a point to a signed barrel row: +n is the n-th visible row
// ABOVE the center selector (larger values), -n below. ok=false outside the
// barrel, inside the center band, or beyond the drawn row range. Mirrors
// drawBarrel's geometry (tickGap spacing around the barrel's vertical center)
// so the clickable rows are exactly the drawn rows.
func (w *ValueWheel) barrelRowAt(x, y int) (int, bool) {
	pt := image.Pt(x, y)
	if w.valRect.Empty() || !pt.In(w.valRect) || pt.In(w.centerH) {
		return 0, false
	}
	gap := w.tickGap()
	if gap <= 0 {
		return 0, false
	}
	centerY := (w.valRect.Min.Y + w.valRect.Max.Y) / 2
	n := int(math.Round(float64(centerY-y) / float64(gap)))
	maxRows := w.valRect.Dy()/(2*gap) + 1
	if n == 0 || n > maxRows || n < -maxRows {
		return 0, false
	}
	return n, true
}

func NewMobileWheelPopup() *MobileWheelPopup { return &MobileWheelPopup{} }

// confirm fires the host's confirm action (centre-selector click).
func (w *ValueWheel) confirm() {
	if w.accept != nil {
		w.accept()
	}
}

// model returns the binding's value model: the explicit Model, else a
// knobModel over the legacy Knob/Badge/Def fields (nil when neither is set).
func (w *ValueWheel) model() WheelModel {
	if w.binding.Model != nil {
		return w.binding.Model
	}
	if w.binding.Knob == nil {
		return nil
	}
	// Refreshed in place every call (no per-frame allocation): the binding is
	// replaced wholesale on every Open.
	if w.km == nil {
		w.km = &knobModel{}
	}
	w.km.k, w.km.b, w.km.def, w.km.discrete = w.binding.Knob, w.binding.Badge, w.binding.Def, w.binding.Discrete
	return w.km
}

func (w *MobileWheelPopup) Open(b WheelBinding, anchor, bounds image.Rectangle, headerH int) {
	w.Bind(b)
	w.anchor = anchor
	dvals := Profile().DensityValues()
	pw, ph := dvals.MobileWheelW, dvals.MobileWheelH
	w.rect = AnchorPopupRect(bounds, anchor, pw, ph, PopupBelow)
	w.layout()
	w.open = true
	if m := w.model(); m != nil {
		w.initialValue = m.Snapshot()
	} else {
		w.initialValue = 0
	}
	w.accept = w.Accept
}

// Bind attaches the wheel to a value binding and resets every gesture /
// cadence / hover state, so a re-bound wheel never carries a half-finished
// drag or a partial notch from the previous value. The confirm action (centre
// click) is cleared; hosts set it with SetConfirm after binding.
func (w *ValueWheel) Bind(b WheelBinding) {
	w.binding = b
	w.dragging = false
	w.dragKind = wheelDragNone
	w.dirty = false
	w.valNotcher = newStepNotcher(w.tickGap())
	w.resNotcher = newStepNotcher(w.tickGap())
	w.wheelNotcher = newStepNotcher(wheelStepEventsPerNotch)
	w.hoverRung = -1
	w.hoverCenter = false
	w.accept = nil
}

// SetConfirm sets the action a click on the centre selector performs.
func (w *ValueWheel) SetConfirm(f func()) { w.accept = f }

// Dirty reports whether the value changed since Bind (or ClearDirty).
func (w *ValueWheel) Dirty() bool { return w.dirty }

// ClearDirty resets the changed-since flag (a host that applied the value).
func (w *ValueWheel) ClearDirty() { w.dirty = false }

// HoverAt refreshes the desktop hover cue for an inline host; points outside
// the wheel clear it.
func (w *ValueWheel) HoverAt(mx, my int) {
	pt := image.Pt(mx, my)
	if !pt.In(w.valRect) && !pt.In(w.resRect) {
		w.hoverRung = -1
		w.hoverCenter = false
		return
	}
	w.hoverAt(pt)
}

// HoverAt implements portalHoverer: refreshes the hovered rung / center-grip
// state from the plain cursor position each frame so the hover tint shows on
// desktop (portal input handlers only run on press edges — see
// OverlayPortal.Update's plain-hover propagation). Cursor outside the popup
// (or the popup closed) clears both.
func (w *MobileWheelPopup) HoverAt(mx, my int) {
	w.hoverRung = -1
	w.hoverCenter = false
	if !w.open {
		return
	}
	pt := image.Pt(mx, my)
	if !pt.In(w.rect) {
		return
	}
	w.ValueWheel.hoverAt(pt)
}

// hoverAt refreshes the hovered rung / centre-grip state for a point already
// known to be inside the host.
func (w *ValueWheel) hoverAt(pt image.Point) {
	w.hoverRung = -1
	w.hoverCenter = false
	if pt.In(w.centerH) {
		w.hoverCenter = true
	}
	for i, rr := range w.rungRects() {
		if pt.In(rr) {
			w.hoverRung = i
			break
		}
	}
}

// layout splits the panel into a barrel column, an optional resolution strip,
// and a center value box. Discrete params hide the strip.
func (w *MobileWheelPopup) layout() {
	inner := w.rect.Inset(SpaceSM)

	// Reserve a title band at the top so the barrel never collides with the title.
	titleH := StyledTextHeight(RoleBody) + SpaceSM*2
	w.titleRect = image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, inner.Min.Y+titleH)
	body := image.Rect(inner.Min.X, inner.Min.Y+titleH, inner.Max.X, inner.Max.Y)
	w.layoutBody(body)
}

// Layout positions the wheel (barrel + optional ladder) inside body. Inline
// hosts call it; MobileWheelPopup reaches it through layout().
func (w *ValueWheel) Layout(body image.Rectangle) { w.layoutBody(body) }

// layoutBody splits body into the barrel column, the optional resolution strip
// and the centre value box.
func (w *ValueWheel) layoutBody(body image.Rectangle) {
	dvals := Profile().DensityValues()
	resW := 0
	if w.hasResolutionStrip() {
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

// rungRects lays the ladder pills vertically inside resRect, COARSEST rung at
// the TOP — bigger numbers on top, matching the value barrel beside it (and
// matching drag/scroll down = coarser, which pulls the bigger steps down
// toward you). Equal heights, compressed when the ladder is long; empty when
// the strip is hidden.
//
// The returned slice stays indexed by LADDER rung (fine→coarse, as
// Badge.Rungs() orders them) and only the geometry is reversed, so drawResStrip,
// the tap hit-test, and hover all follow from this one mapping.
func (w *ValueWheel) rungRects() []image.Rectangle {
	m := w.model()
	if w.resRect.Empty() || m == nil {
		return nil
	}
	n := m.LadderLen()
	if n == 0 {
		return nil
	}
	inner := w.resRect.Inset(SpaceXS)
	gap := 2
	h := (inner.Dy() - gap*(n-1)) / n
	if h < 12 {
		h = 12
		gap = 1
	}
	out := make([]image.Rectangle, n)
	y := inner.Min.Y
	for slot := 0; slot < n; slot++ {
		// Slot 0 is the top pill and holds the LAST (coarsest) ladder rung.
		out[n-1-slot] = image.Rect(inner.Min.X, y, inner.Max.X, y+h)
		y += h + gap
	}
	return out
}

func (w *MobileWheelPopup) Close() { w.open = false; w.dragging = false; w.dragKind = wheelDragNone }

// Accept persists the current edit: if the value changed since Open it fires
// OnCommit once (recording a single undo step), then closes. This is the
// terminal action for Enter, tapping outside, re-tapping the opener, and
// navigating away — every dismissal EXCEPT Esc. A no-op when nothing changed.
func (w *MobileWheelPopup) Accept() {
	if !w.open {
		return
	}
	if w.dirty && w.binding.OnCommit != nil {
		w.binding.OnCommit()
	}
	w.Close()
}

// Cancel discards the edit: it restores the value snapshotted at Open, re-applies
// it via OnChange (so the source of truth and live audio revert to their previous
// state), does NOT commit, then closes. This is the terminal action for Esc.
func (w *MobileWheelPopup) Cancel() {
	if !w.open {
		return
	}
	if m := w.model(); w.dirty && m != nil {
		m.Restore(w.initialValue)
		if w.binding.OnCancel != nil {
			w.binding.OnCancel()
		} else if w.binding.OnChange != nil {
			w.binding.OnChange()
		}
	}
	w.Close()
}
func (w *MobileWheelPopup) IsOpen() bool            { return w.open }
func (w *MobileWheelPopup) Rect() image.Rectangle   { return w.rect }
func (w *MobileWheelPopup) Anchor() image.Rectangle { return w.anchor }
func (w *ValueWheel) IsDragging() bool              { return w.dragging && w.dragKind == wheelDragValue }

// IsDraggingAny reports whether ANY drag (value OR resolution strip) is active.
// The portal hit handler uses this to claim capture so move events keep flowing
// to the popup; the narrower IsDragging() stays value-only for row-scroll isolation.
func (w *ValueWheel) IsDraggingAny() bool { return w.dragging }

// HandleWheel applies a mouse-wheel / trackpad two-finger scroll. Scroll
// MAGNITUDE is deliberately ignored (a two-finger flick reports a large delta
// per event, which used to fly the value); instead each event contributes one
// unit of direction to a notcher, so it takes wheelStepEventsPerNotch events to
// advance one step — slow and controlled, matching the clicky drag. Over the
// resolution strip it shifts the rung at the same cadence. Returns true if it
// consumed the event. Value changes preview live (OnChange); the undo commit is
// deferred to Accept (Enter / tap-away), like the drag path.
func (w *MobileWheelPopup) HandleWheel(mx, my, steps int) bool {
	if !w.open {
		return false
	}
	return w.ValueWheel.HandleWheel(mx, my, steps)
}

// HandleWheel applies a mouse-wheel / trackpad scroll to the wheel (see
// MobileWheelPopup.HandleWheel for the pacing and direction contract).
func (w *ValueWheel) HandleWheel(mx, my, steps int) bool {
	if steps == 0 {
		return false
	}
	dir := 1
	if steps < 0 {
		dir = -1
	}
	n := w.wheelNotcher.add(dir) // paced: magnitude-independent, events-per-step
	m := w.model()
	overStrip := !w.resRect.Empty() && image.Pt(mx, my).In(w.resRect) && m != nil && m.HasLadder()
	if n == 0 {
		return true // consumed, but not enough scroll has accumulated for a step yet
	}
	if overStrip {
		changed := false
		// Match the resolution DRAG (and the value barrel): a DOWNWARD scroll is a
		// positive step on the target (n>0) and must COARSEN (bigger step number),
		// like the value wheel where a downward scroll raises the shown number.
		// Scroll UP (n<0) finens. (This mirrors applyResDelta, where drag-down —
		// steps<0 — already coarsens; the two paths now agree.)
		for i := 0; i < n; i++ {
			changed = m.AdjustLadder(-1) || changed // scroll down => coarser
		}
		for i := 0; i > n; i-- {
			changed = m.AdjustLadder(1) || changed // scroll up => finer
		}
		if changed {
			if w.binding.OnResolution != nil {
				w.binding.OnResolution()
			}
		}
		return true
	}
	if m == nil {
		return false
	}
	// Same barrel "down = higher" direction as the finger drag: a DOWNWARD scroll
	// must RAISE the value (higher numbers descend to the center selector). The tree
	// hands the popup ebiten.Wheel's raw int(wy) as steps. EMPIRICAL PLATFORM FACT
	// (confirmed on the target device): a downward two-finger/wheel scroll arrives as
	// POSITIVE int(wy), so n>0 for a downward scroll and we apply the RAW n to
	// increase. (The wheel "down" sign is natural-scroll/platform dependent; we match
	// the user's device. The finger drag reaches the same "down = increase" via
	// delta=lastY-my; both are locked together by TestWheelDownGesturesAllIncrease.)
	//
	// Shift+scroll applies ONE ladder rung finer than the active resolution
	// WITHOUT persisting the rung (mirrors nudgeKnobByWheel's fine path).
	fine := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	if fine && m.HasLadder() {
		m.StepFine(n)
	} else {
		m.Step(n)
	}
	// Live preview only; the undo commit is deferred to Accept (Enter / tap-away).
	w.dirty = true
	if w.binding.OnChange != nil {
		w.binding.OnChange()
	}
	return true
}

// HandleInput routes a pointer event. Returns true if the popup consumed it.
func (w *MobileWheelPopup) HandleInput(mx, my int, pressed bool) bool {
	if !w.open {
		return false
	}
	return w.ValueWheel.HandleInput(mx, my, pressed)
}

// HandleInput routes a pointer event to the barrel / ladder. Returns true when
// the wheel consumed it.
func (w *ValueWheel) HandleInput(mx, my int, pressed bool) bool {
	if !pressed {
		// A release that never left the tap slop is a CLICK (drags that start
		// in the same spot are unaffected — any real movement sets pressMoved):
		//  - on the CENTER value box it confirms (the host's accept action:
		//    the popup Accepts, the go-to card seeks) — the uniform "click the
		//    middle to confirm" contract, on every wheel.
		//  - on a visible barrel ROW it selects that row's value directly
		//    (live preview via OnChange; the undo commit stays deferred to
		//    Accept, like any other edit).
		if w.dragging && w.dragKind == wheelDragValue && !w.pressMoved {
			if w.pressPt.In(w.centerH) {
				w.dragging = false
				w.dragKind = wheelDragNone
				w.confirm()
				return true
			}
			if n, ok := w.barrelRowAt(w.pressPt.X, w.pressPt.Y); ok {
				if m := w.model(); m != nil && m.SelectRow(n) {
					w.dirty = true
					if w.binding.OnChange != nil {
						w.binding.OnChange()
					}
				}
			}
		}
		// Releasing a drag ends the gesture but does NOT commit — the undo
		// commit is deferred to Accept (Enter / tap-away). See Accept/Cancel.
		w.dragging = false
		w.dragKind = wheelDragNone
		return true
	}
	pt := image.Pt(mx, my)
	if !w.dragging {
		// Press: decide which sub-control this gesture owns.
		switch {
		case !w.resRect.Empty() && pt.In(w.resRect):
			// Tap directly on a rung pill jumps straight to that resolution and
			// persists it (OnResolution). A press in the strip gutter (outside any
			// rung) still starts the legacy clicky res-drag below.
			if m := w.model(); m != nil && m.HasLadder() {
				for i, rr := range w.rungRects() {
					if pt.In(rr) {
						if m.SetLadderIndex(i) {
							if w.binding.OnResolution != nil {
								w.binding.OnResolution()
							}
						}
						return true
					}
				}
			}
			w.dragging = true
			w.dragKind = wheelDragRes
			w.lastY = my
			w.resNotcher.setPxPerNotch(w.tickGap())
			w.resNotcher.reset()
		default:
			w.dragging = true
			w.dragKind = wheelDragValue
			w.lastY = my
			w.pressPt = pt
			w.pressMoved = false
			w.valNotcher.setPxPerNotch(w.tickGap())
			w.valNotcher.reset()
		}
		return true
	}
	// Continue drag.
	switch w.dragKind {
	case wheelDragValue:
		if dy := my - w.pressPt.Y; dy > wheelTapSlopPx || dy < -wheelTapSlopPx {
			w.pressMoved = true // past the tap slop: this gesture is a drag, not a click
		}
		delta := w.lastY - my // finger down (y increases) => negative delta; see applyValueDelta
		w.lastY = my
		if w.applyValueDelta(delta) { // only when a detent actually clicked over
			w.dirty = true
			if w.binding.OnChange != nil {
				w.binding.OnChange()
			}
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
// params advance one item per notch. Returns true if at least one detent clicked
// over.
//
// Direction: the barrel draws higher values at the top, lower at the bottom, so
// dragging the finger DOWN (delta<0) pulls the higher rows toward the center
// selector and must INCREASE the value. Both branches therefore invert delta so
// that finger-down => positive steps => value up.
func (w *ValueWheel) applyValueDelta(delta int) bool {
	m := w.model()
	if m == nil {
		return false
	}
	// Drag down (delta<0) => -delta>0 => steps>0 => the value rises (enum: the
	// index advances). One notch == one detent at the active resolution.
	steps := w.valNotcher.add(-delta)
	if steps == 0 {
		return false
	}
	m.Step(steps)
	return true
}

// applyResDelta drags the resolution strip across the KnobStepBadge rungs and
// keeps the knob's StepMul in sync. One rung shift per tick-gap of travel.
func (w *ValueWheel) applyResDelta(delta int) {
	m := w.model()
	if m == nil || !m.HasLadder() {
		return
	}
	// Same clicky cadence as the value drag: one rung per notch of travel.
	steps := w.resNotcher.add(delta)
	changed := false
	for i := 0; i < steps; i++ { // drag up = finer
		if m.AdjustLadder(1) {
			changed = true
		}
	}
	for i := 0; i > steps; i-- { // drag down = coarser
		if m.AdjustLadder(-1) {
			changed = true
		}
	}
	if changed && w.binding.OnResolution != nil {
		w.binding.OnResolution()
	}
}

// ── keyboard editing (KeyboardKeyClaimant, see keyboard_key_claim.go) ────────
//
// Up/Down step the VALUE by one resolution step; Left/Right move the step
// RESOLUTION one ladder rung. Direction matches the Synth tab's armed control
// (ArrowRight = coarser), so both value editors read the same way. Every other
// key falls through to the normal shortcut router: Space still plays, [ ] still
// zoom, Enter/Esc keep their Accept/Cancel meaning from the portal overlay.

// ClaimedKeys implements KeyboardKeyClaimant: the vertical arrows while open,
// plus the horizontal pair only when the resolution ladder is actually ON
// SCREEN. The condition mirrors layout()'s resW gate exactly — claiming
// Left/Right for a rung the user cannot see would take arrow-pan away to move
// nothing.
func (w *MobileWheelPopup) ClaimedKeys() []ebiten.Key {
	if w == nil || !w.open {
		return nil
	}
	return w.ValueWheel.ClaimedKeys()
}

// ClaimedKeys: the vertical arrows, plus the horizontal pair only while the
// resolution ladder is on screen.
func (w *ValueWheel) ClaimedKeys() []ebiten.Key {
	if w == nil {
		return nil
	}
	if w.hasResolutionStrip() {
		return valueAndResolutionStepKeys()
	}
	return valueStepKeys()
}

// hasResolutionStrip reports whether the resolution ladder is laid out. Single
// source for layout() and ClaimedKeys, so the claim can't drift from the strip.
func (w *ValueWheel) hasResolutionStrip() bool {
	m := w.model()
	return m != nil && m.HasLadder()
}

// HandleClaimedKeys implements KeyboardKeyClaimant. One key acts per frame (the
// switch), so the alias spellings real Ebiten collapses cannot double-fire.
func (w *MobileWheelPopup) HandleClaimedKeys() bool {
	if w == nil || !w.open {
		return false
	}
	return w.ValueWheel.HandleClaimedKeys()
}

// HandleClaimedKeys steps the value (Up/Down) or the resolution (Left/Right).
// One key acts per frame, so alias spellings cannot double-fire.
func (w *ValueWheel) HandleClaimedKeys() bool {
	if w == nil {
		return false
	}
	switch {
	case anyKeyJustPressed(keysStepUp...):
		return w.StepValue(+1)
	case anyKeyJustPressed(keysStepDown...):
		return w.StepValue(-1)
	case w.hasResolutionStrip() && anyKeyJustPressed(keysStepRight...):
		return w.StepResolution(+1)
	case w.hasResolutionStrip() && anyKeyJustPressed(keysStepLeft...):
		return w.StepResolution(-1)
	}
	return false
}

// StepValue advances the value by exactly ONE resolution step in dir (+1/-1) —
// the keyboard's discrete counterpart to a drag notch, which is why it does NOT
// go through valNotcher (a keypress must move the value on the frame it lands,
// not after enough travel accumulates). Live-previews via OnChange; the undo
// commit stays deferred to Accept, exactly like the drag and wheel paths.
func (w *ValueWheel) StepValue(dir int) bool {
	m := w.model()
	if m == nil || dir == 0 {
		return false
	}
	if !m.Step(sign(dir)) {
		return false // already at Min/Max
	}
	w.dirty = true
	if w.binding.OnChange != nil {
		w.binding.OnChange()
	}
	return true
}

// StepResolution moves the step-resolution ladder one rung: +1 = COARSER (a
// bigger step number), matching synthResolutionBinding.stepBy so both value
// editors read the same way. Badge.AdjustResolution(+1) is FINER, so the delta
// is inverted here.
func (w *ValueWheel) StepResolution(dir int) bool {
	m := w.model()
	if m == nil || dir == 0 || !m.AdjustLadder(-sign(dir)) {
		return false
	}
	if w.binding.OnResolution != nil {
		w.binding.OnResolution()
	}
	return true
}
