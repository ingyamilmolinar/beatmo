package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The resolution lane exists only for continuous params; edits coalesce into one debounced undo step.
type ArmedControlEditor struct {
	instID instrument.ID
	idx    int

	onEditFn func()

	valWheel *BarrelWheel
	resWheel *BarrelWheel // nil when the control has no resolution ladder

	card    image.Rectangle
	valRect image.Rectangle
	resRect image.Rectangle
	active  *BarrelWheel // wheel currently captured by a drag
}

// Panel-agnostic: the Synth and Sampler tabs both build one through the same factories, so their editing is
// identical by construction. A nil resB means no resolution ladder.
func newArmedControlEditor(instID instrument.ID, idx int, valB, resB barrelBinding, onEdit func()) *ArmedControlEditor {
	e := &ArmedControlEditor{instID: instID, idx: idx, onEditFn: onEdit}
	detent := Profile().DensityValues().SynthWheelRowH
	e.valWheel = NewBarrelWheel()
	e.valWheel.Bind(valB, detent)
	if resB != nil {
		e.resWheel = NewBarrelWheel()
		e.resWheel.Bind(resB, detent)
	}
	return e
}

func (e *ArmedControlEditor) value() *BarrelWheel { return e.valWheel }
func (e *ArmedControlEditor) res() *BarrelWheel   { return e.resWheel }

// Just enough to read as two columns, with a divider hairline in it.
const synthLaneGapW = SpaceXS

// The gap keeps the two label stacks from visually merging.
func (e *ArmedControlEditor) SetCardRect(card image.Rectangle) {
	e.card = card
	v := Profile().DensityValues()
	// Reserve a header band at the top (param name) == one wheel row.
	body := card
	body.Min.Y = card.Min.Y + v.SynthWheelRowH
	if e.resWheel != nil {
		laneW := v.SynthWheelLaneW
		if laneW > body.Dx()/2 {
			laneW = body.Dx() / 2
		}
		gap := synthLaneGapW
		if laneW+gap >= body.Dx() {
			gap = 0 // no room for a gutter on a very narrow card
		}
		e.resRect = image.Rect(body.Min.X, body.Min.Y, body.Min.X+laneW, body.Max.Y)
		e.valRect = image.Rect(body.Min.X+laneW+gap, body.Min.Y, body.Max.X, body.Max.Y)
	} else {
		e.resRect = image.Rectangle{}
		e.valRect = body
	}
}

func (e *ArmedControlEditor) ValueLaneRect() image.Rectangle { return e.valRect }
func (e *ArmedControlEditor) ResLaneRect() image.Rectangle   { return e.resRect }

// Empty with no resolution lane or no room for a gap.
func (e *ArmedControlEditor) LaneGapRect() image.Rectangle {
	if e.resRect.Empty() || e.valRect.Empty() {
		return image.Rectangle{}
	}
	if e.valRect.Min.X <= e.resRect.Max.X {
		return image.Rectangle{}
	}
	return image.Rect(e.resRect.Max.X, e.resRect.Min.Y, e.valRect.Min.X, e.resRect.Max.Y)
}

// The value lane is the default, so a press just outside the narrow res lane still edits the value.
func (e *ArmedControlEditor) laneAt(x int) *BarrelWheel {
	if e.resWheel != nil && !e.resRect.Empty() && x < e.resRect.Max.X {
		return e.resWheel
	}
	return e.valWheel
}

func (e *ArmedControlEditor) BeginDrag(x, y int) {
	e.active = e.laneAt(x)
	if e.active != nil {
		e.active.BeginDrag(y)
	}
}

func (e *ArmedControlEditor) DragTo(y int) bool {
	if e.active == nil {
		return false
	}
	if e.active.DragTo(y) {
		e.onEdit()
		return true
	}
	return false
}

func (e *ArmedControlEditor) EndDrag() {
	if e.active != nil {
		e.active.EndDrag()
	}
	e.active = nil
}

func (e *ArmedControlEditor) Wheel(x, y, steps int) bool {
	w := e.laneAt(x)
	if w == nil || steps == 0 {
		return false
	}
	// Positive steps (scroll down on target) = +1 = increase / coarser.
	changed := w.ImpulseNotch(sign(steps))
	if changed {
		e.onEdit()
	}
	return changed
}

// A keypress must change the value synchronously, unlike a kinetic notch that resolves over later Ticks.
func (e *ArmedControlEditor) StepValue(dir int) bool {
	if e.valWheel == nil {
		return false
	}
	ok := e.valWheel.StepImmediate(sign(dir))
	if ok {
		e.onEdit()
	}
	return ok
}

func (e *ArmedControlEditor) StepResolution(dir int) bool {
	if e.resWheel == nil {
		return false
	}
	ok := e.resWheel.StepImmediate(sign(dir))
	if ok {
		e.onEdit()
	}
	return ok
}

// Returns true while either wheel animates.
func (e *ArmedControlEditor) Tick() bool {
	anim := false
	if e.valWheel != nil && e.valWheel.Tick() {
		anim = true
		e.onEdit()
	}
	if e.resWheel != nil && e.resWheel.Tick() {
		anim = true
		e.onEdit()
	}
	return anim
}

// onEdit fires the owning panel's live-propagate + debounced-commit closure.
func (e *ArmedControlEditor) onEdit() {
	if e.onEditFn != nil {
		e.onEditFn()
	}
}

// Without one, Left/Right stay with the grid.
func (e *ArmedControlEditor) HasResolutionLane() bool { return e != nil && e.resWheel != nil }
