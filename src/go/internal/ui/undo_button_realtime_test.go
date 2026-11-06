//go:build test

package ui

import (
	"image/color"
	"testing"
)

// TestSynthControlUndoButtonReactsSameFrame proves the transport Undo button reacts
// to a Synth-tab value edit in REAL TIME. Task-9 value edits go through the ARMED
// value lane (arm the control, then spin its value lane); the single undo step
// commits when the debounce elapses (inside an Update's synthTabUpdate). On the
// very frame the commit lands, that frame's Draw must already render the button
// lit — not one or more frames later.
func TestSynthControlUndoButtonReactsSameFrame(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()

	img := newTrackedImage("test.knobrealtime", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	dv := g.drum
	instID := dv.synthTabActiveInstrument()
	k := dv.SynthTabControls()[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatal("knob rect empty")
	}

	// Arm the control (Task 9: a value edit now requires arming first — the
	// not-armed wheel scrolls the row instead of nudging the value).
	dv.armSynthControl(instID, idx)
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	e.SetCardRect(r)
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: instID}
	vx, vy := e.ValueLaneRect().Min.X+2, r.Min.Y+r.Dy()/2

	g.undoManager.OnExternalLoad()
	tz := dv.transportZone

	// Baseline: nothing to undo -> greyed.
	g.Update()
	g.Draw(img)
	if iconColorOf(tz.undoBtn) != color.Color(colTextDisabled) {
		t.Fatalf("precondition: undo should be greyed before any edit, got %v", iconColorOf(tz.undoBtn))
	}

	// Spin a few wheel notches on the armed value lane (mutates live audio,
	// records nothing yet).
	for s := 0; s < 3; s++ {
		dv.eqPanelZone.Invalidate()
		if res := h.OnWheel(vx, vy, +1); res != InputConsumed {
			t.Fatalf("armed wheel notch %d should be consumed, got %v", s, res)
		}
		g.Update()
	}
	if g.undoManager.CanUndo() {
		t.Fatal("undo step must NOT be recorded mid-spin (before the debounce elapses)")
	}
	// Settle the kinetic barrel so the value lane actually crosses a detent
	// (SHARED FACTS: ImpulseNotch only resolves into a value change via Tick).
	for i := 0; i < 240 && e.Tick(); i++ {
	}

	// Run the debounce down; the commit frame's Draw must show the button lit.
	committed := false
	for i := 0; i < wheelNudgeCommitFrames+8; i++ {
		g.Update()
		if g.undoManager.CanUndo() {
			committed = true
			g.Draw(img) // SAME frame as the commit
			if got := iconColorOf(tz.undoBtn); got != color.Color(colTextSecondary) {
				t.Fatalf("undo button did not react on the commit frame (got %v, want lit %v)", got, color.Color(colTextSecondary))
			}
			break
		}
	}
	if !committed {
		t.Fatal("wheel nudge never recorded an undo step after the debounce elapsed")
	}
}
