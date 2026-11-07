//go:build test

package ui

import "testing"

// Unarmed, a wheel scrolls the grid and a still tap arms; armed, input spins the hovered lane and a second still
// tap locks it.

// Scrolls the section's grid and leaves the value alone.
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

// A still tap arms, a wheel over the value lane edits, a second still tap locks.
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

// A drag past the tap slop (10 px) but short of a whole detent (22-32 px) does not lock on release: pointer
// displacement decides "moved", not a detent crossing.
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

	// Past tapMaxMovePx, under the smallest SynthWheelRowH (22 px).
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
