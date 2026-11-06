//go:build test

package ui

import "testing"

// synth_armed_input_test.go — Task 9. Pins the armed-model wiring of
// synthControlHitAdapter: not-armed input scrolls the section grid / arms the
// control on a stationary tap; armed input spins the hovered barrel lane and
// a second stationary tap locks it back out.

// TestNotArmedWheelScrollsRow: a wheel notch over a control that is NOT armed
// must scroll the section's ControlGrid and must NOT change the control's
// value (the "desktop behavior change" — wheel used to nudge the value).
func TestNotArmedWheelScrollsRow(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	layoutSynthTab(t, g)
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("no selected synth section with knobs")
	}
	grid := dv.sectionGrid(sec.id)
	if grid == nil || !grid.HasScroll() {
		t.Skip("section doesn't overflow; no scroll to test")
	}
	idx := sec.knobIdxs[0]
	instID := dv.synthTabActiveInstrument()
	if _, armed := dv.synthControlArmed(instID); armed {
		t.Fatal("control unexpectedly armed before the test")
	}
	first := grid.Scroll().VS.First
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: instID}
	r := dv.instEditorControls[idx].Rect()
	if r.Empty() {
		t.Fatal("control has no rect after layout")
	}
	before := dv.instEditorControls[idx].Value

	h.OnWheel(r.Min.X+2, r.Min.Y+2, -3)

	if grid.Scroll().VS.First == first {
		t.Fatal("not-armed wheel over a control must scroll the row")
	}
	if dv.instEditorControls[idx].Value != before {
		t.Fatal("not-armed wheel must NOT change the value")
	}
}

// TestTapArmsThenWheelEditsValue: a stationary press+release arms the
// control (was: opens the value-wheel popup); a wheel over the armed
// editor's value lane then changes the value; a second stationary tap locks
// the control back out.
func TestTapArmsThenWheelEditsValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	layoutSynthTab(t, g)
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("no selected synth section with knobs")
	}
	idx := sec.knobIdxs[0]
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: inst}
	r := dv.instEditorControls[idx].Rect()
	if r.Empty() {
		t.Fatal("control has no rect after layout")
	}

	// Stationary press+release = arm.
	h.OnPress(r.Min.X+2, r.Min.Y+2)
	h.OnRelease(r.Min.X+2, r.Min.Y+2)
	if aidx, armed := dv.synthControlArmed(inst); !armed || aidx != idx {
		t.Fatalf("stationary tap should arm the control (idx=%d armed=%v got idx=%d)", idx, armed, aidx)
	}
	if dv.instEditorArmedEditor == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}

	before := dv.instEditorControls[idx].Value
	// Now the wheel edits the value (right lane).
	e := dv.instEditorArmedEditor
	e.SetCardRect(r) // ensure lane rects
	vx := e.ValueLaneRect().Min.X + 2
	h.OnWheel(vx, r.Min.Y+r.Dy()/2, +2)
	for i := 0; i < 240 && e.Tick(); i++ {
	}
	if dv.instEditorControls[idx].Value <= before {
		t.Fatal("armed wheel should change the value")
	}

	// Tap again (stationary) → lock.
	h.OnPress(vx, r.Min.Y+2)
	h.OnRelease(vx, r.Min.Y+2)
	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("second stationary tap should lock the control")
	}
}

// TestSynthControlArmedSubDetentDragStaysArmed: Finding 1 regression. An
// armed drag that travels PAST the tap slop (tapMaxMovePx=10) but hasn't
// crossed a whole barrel detent (SynthWheelRowH=22-32) must NOT lock the
// control on release — only cumulative pointer displacement decides "moved",
// not whether ArmedControlEditor.DragTo/BarrelWheel reported a detent
// crossing.
func TestSynthControlArmedSubDetentDragStaysArmed(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	layoutSynthTab(t, g)
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("no selected synth section with knobs")
	}
	idx := sec.knobIdxs[0]
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: inst}
	r := dv.instEditorControls[idx].Rect()
	if r.Empty() {
		t.Fatal("control has no rect after layout")
	}

	// Arm via a stationary tap first.
	h.OnPress(r.Min.X+2, r.Min.Y+2)
	h.OnRelease(r.Min.X+2, r.Min.Y+2)
	if aidx, armed := dv.synthControlArmed(inst); !armed || aidx != idx {
		t.Fatalf("stationary tap should arm the control (idx=%d armed=%v got idx=%d)", idx, armed, aidx)
	}

	// Sanity: the sub-detent drag distance must clear tapMaxMovePx but stay
	// under the smallest SynthWheelRowH across densities (22px).
	const dragDelta = 14
	if dragDelta <= tapMaxMovePx {
		t.Fatalf("dragDelta=%d must exceed tapMaxMovePx=%d", dragDelta, tapMaxMovePx)
	}

	startY := r.Min.Y + 2
	h.OnPress(r.Min.X+2, startY)
	h.OnDrag(r.Min.X+2, startY+dragDelta)
	h.OnRelease(r.Min.X+2, startY+dragDelta)

	if aidx, armed := dv.synthControlArmed(inst); !armed || aidx != idx {
		t.Fatalf("a sub-detent armed drag must NOT lock the control (idx=%d armed=%v got idx=%d)", idx, armed, aidx)
	}
}
