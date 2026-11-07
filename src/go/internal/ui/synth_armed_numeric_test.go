//go:build test

package ui

import "testing"

// Typed entry from the armed control is off: any still click on the card locks it, a drag still spins the lane.
// openArmedNumericEditor stays, one line away from re-enabling in OnRelease.

// Locks, and opens no numeric editor.
func TestArmedReadoutTapLocks(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	growSynthPanelForTest(t, g)
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("no selected synth section with knobs")
	}
	idx := sec.knobIdxs[0]
	dv.armSynthControl(inst, idx)
	layoutSynthTab(t, g)

	r := dv.instEditorControls[idx].Rect()
	if r.Empty() {
		t.Fatal("control has no rect after layout")
	}
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	e.SetCardRect(r)
	vr := e.ValueLaneRect()
	if vr.Empty() {
		t.Fatal("value lane rect should be non-empty")
	}
	cx := vr.Min.X + vr.Dx()/2
	cy := vr.Min.Y + vr.Dy()/2

	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: inst}
	h.OnPress(cx, cy)
	h.OnRelease(cx, cy)

	if dv.KeyboardClaimed() {
		t.Fatal("value-lane readout tap must NOT start text entry (values are set with the barrel)")
	}
	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("a stationary tap on the value readout must LOCK the control")
	}
}

// The header band locks too.
func TestArmedTapOutsideReadoutLocks(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	growSynthPanelForTest(t, g)
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("no selected synth section with knobs")
	}
	idx := sec.knobIdxs[0]
	dv.armSynthControl(inst, idx)
	layoutSynthTab(t, g)

	r := dv.instEditorControls[idx].Rect()
	if r.Empty() {
		t.Fatal("control has no rect after layout")
	}
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	e.SetCardRect(r)

	px, py := r.Min.X+2, r.Min.Y+2

	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: inst}
	h.OnPress(px, py)
	h.OnRelease(px, py)

	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("stationary tap on the armed card should LOCK the control")
	}
	if dv.KeyboardClaimed() {
		t.Fatal("tap must not start text entry (values are set with the barrel)")
	}
}
