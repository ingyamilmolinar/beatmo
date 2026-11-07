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

// The popup stores NO value of its own.
type WheelBinding struct {
	// nil adapts the Knob/Badge/Def fields through knobModel.
	Model    WheelModel
	Knob     *Knob
	Badge    *KnobStepBadge // nil for discrete/enum params (no resolution strip)
	Def      audio.ParamDef
	Discrete bool // enum or Step>0: step detents instead of endless accumulation
	OnChange func()
	// For a model that converts or clamps (dB ↔ linear, a floor) and so cannot round-trip the opening
	// value: restore it directly.
	OnCancel     func()
	OnCommit     func()
	OnResolution func()
	Title        func() string
	Gloss        func() string // optional one-line PlainEnglish purpose ("bright or dull")
	Accent       func() color.Color
}

// No frame or title, so it can sit inside any card; MobileWheelPopup adds the frame and the
// transactional Accept/Cancel. Owns no value: everything goes through the WheelModel.
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

	// A release that never passed the slop is a CLICK: on a visible row it selects that value.
	pressPt    image.Point
	pressMoved bool

	dirty bool // a value change happened since the host last reset it

	// Fired by a click on the centre selector; nil = inert.
	accept func()

	// Portal input only runs on press edges, so hover is refreshed per frame via HoverAt.
	hoverRung   int  // rung index under the cursor, -1 when none
	hoverCenter bool // cursor is over the center value box

	km *knobModel // cached legacy adapter (see model)
}

// Anchored to its opener; live preview, one commit on Accept, restore on Cancel.
type MobileWheelPopup struct {
	ValueWheel
	open      bool
	rect      image.Rectangle
	anchor    image.Rectangle
	titleRect image.Rectangle // reserved title band at the panel top

	// Turning previews live; the undo commit waits for Accept (Enter, tap-away, navigate-away). Esc
	// restores initialValue.
	initialValue float64 // model value snapshotted at Open (for Cancel)
}

// Magnitude is ignored (a trackpad flick reports a huge delta); pacing by event count keeps it controlled.
const wheelStepEventsPerNotch = 3

// Well below tickGap, so a sub-slop gesture can never fire both a detent and a row jump.
const wheelTapSlopPx = 6

// +n is the n-th row ABOVE centre (larger values). Mirrors drawBarrel's geometry, so the clickable
// rows are exactly the drawn ones.
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

func (w *ValueWheel) model() WheelModel {
	if w.binding.Model != nil {
		return w.binding.Model
	}
	if w.binding.Knob == nil {
		return nil
	}
	// Refreshed in place, never allocated per frame.
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

// Resets every gesture, cadence and hover state, so a re-bound wheel never carries a half-finished
// drag; hosts set the confirm action afterwards.
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

// Points outside the wheel clear it.
func (w *ValueWheel) HoverAt(mx, my int) {
	pt := image.Pt(mx, my)
	if !pt.In(w.valRect) && !pt.In(w.resRect) {
		w.hoverRung = -1
		w.hoverCenter = false
		return
	}
	w.hoverAt(pt)
}

// Portal input handlers only run on press edges, so hover needs this per-frame pass.
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

// Discrete params hide the strip.
func (w *MobileWheelPopup) layout() {
	inner := w.rect.Inset(SpaceSM)

	// Reserve a title band at the top so the barrel never collides with the title.
	titleH := StyledTextHeight(RoleBody) + SpaceSM*2
	w.titleRect = image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, inner.Min.Y+titleH)
	body := image.Rect(inner.Min.X, inner.Min.Y+titleH, inner.Max.X, inner.Max.Y)
	w.layoutBody(body)
}

func (w *ValueWheel) Layout(body image.Rectangle) { w.layoutBody(body) }

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

// COARSEST at the top, like bigger numbers on the barrel beside it. Indexed by ladder rung
// (fine→coarse) with only the geometry reversed, so draw, hit-test and hover share one mapping.
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

// The terminal action for every dismissal EXCEPT Esc: one commit (one undo step) if the value
// changed, then close.
func (w *MobileWheelPopup) Accept() {
	if !w.open {
		return
	}
	if w.dirty && w.binding.OnCommit != nil {
		w.binding.OnCommit()
	}
	w.Close()
}

// Restores the snapshot through OnChange, so the source of truth and live audio revert. No commit.
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

// Capture needs any drag; IsDragging() stays value-only for row-scroll isolation.
func (w *ValueWheel) IsDraggingAny() bool { return w.dragging }

// Magnitude is ignored (a flick used to fly the value): each event is one unit of direction, and
// it takes wheelStepEventsPerNotch events per step. The commit waits for Accept.
func (w *MobileWheelPopup) HandleWheel(mx, my, steps int) bool {
	if !w.open {
		return false
	}
	return w.ValueWheel.HandleWheel(mx, my, steps)
}

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
		// A DOWNWARD scroll coarsens, matching the drag and the barrel.
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
	// A DOWNWARD scroll raises the value, like the finger drag; on the target devices it arrives as
	// positive wy, so the raw n applies. Shift scrolls one rung finer without persisting it.
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

func (w *ValueWheel) HandleInput(mx, my int, pressed bool) bool {
	if !pressed {
		// A release inside the slop is a CLICK: on the centre it confirms (every wheel's contract), on a
		// visible row it selects that value (committed later, on Accept).
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
		// Ending a drag does not commit; Accept does.
		w.dragging = false
		w.dragKind = wheelDragNone
		return true
	}
	pt := image.Pt(mx, my)
	if !w.dragging {
		// Press: decide which sub-control this gesture owns.
		switch {
		case !w.resRect.Empty() && pt.In(w.resRect):
			// A tap on a rung jumps to it and persists it; a press in the gutter still starts a res-drag.
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

// One detent per notch of the cadence, so a tiny drag never flies through values. Dragging DOWN
// pulls the higher rows to centre, so it increases the value: delta is inverted.
func (w *ValueWheel) applyValueDelta(delta int) bool {
	m := w.model()
	if m == nil {
		return false
	}
	steps := w.valNotcher.add(-delta)
	if steps == 0 {
		return false
	}
	m.Step(steps)
	return true
}

// One rung per tick-gap of travel; keeps the knob's StepMul in sync.
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

// Up/Down step the value, Left/Right the resolution (Right = coarser), like the Synth tab's armed
// control. Every other key falls through: Space plays, Enter/Esc keep Accept/Cancel.

// Left/Right only while the ladder is ON SCREEN (the same gate as layout): claiming them for an
// invisible rung would take arrow-pan away for nothing.
func (w *MobileWheelPopup) ClaimedKeys() []ebiten.Key {
	if w == nil || !w.open {
		return nil
	}
	return w.ValueWheel.ClaimedKeys()
}

func (w *ValueWheel) ClaimedKeys() []ebiten.Key {
	if w == nil {
		return nil
	}
	if w.hasResolutionStrip() {
		return valueAndResolutionStepKeys()
	}
	return valueStepKeys()
}

// The one source for layout() and ClaimedKeys, so the claim cannot drift from the strip.
func (w *ValueWheel) hasResolutionStrip() bool {
	m := w.model()
	return m != nil && m.HasLadder()
}

// One key per frame, so the alias spellings cannot double-fire.
func (w *MobileWheelPopup) HandleClaimedKeys() bool {
	if w == nil || !w.open {
		return false
	}
	return w.ValueWheel.HandleClaimedKeys()
}

// One key per frame, so the alias spellings cannot double-fire.
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

// Not through valNotcher: a keypress must move the value on the frame it lands. The commit still
// waits for Accept.
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

// +1 = coarser, matching the Synth tab; Badge.AdjustResolution(+1) is finer, hence the inversion.
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
