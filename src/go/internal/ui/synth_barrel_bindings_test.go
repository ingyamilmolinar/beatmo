package ui

import "testing"

func TestSynthValueBinding_StepChangesKnob(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	instID := dv.synthTabActiveInstrument()
	// Arm-independent: bind directly to control 0 and step it up.
	if len(dv.instEditorControls) == 0 {
		t.Skip("no synth controls for this instrument")
	}
	before := dv.instEditorControls[0].Value
	b := synthValueBinding{dv: dv, idx: 0, instID: instID}
	if !b.stepBy(+3) {
		t.Fatal("stepBy should report a change")
	}
	if dv.instEditorControls[0].Value <= before {
		t.Fatalf("value should increase: before=%v after=%v", before, dv.instEditorControls[0].Value)
	}
	if b.labelAt(0) == "" {
		t.Fatal("labelAt(0) must render the current value")
	}
}

func TestSynthResolutionBinding_WalksLadderAndPersists(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	instID := dv.synthTabActiveInstrument()
	// Find a control that has a step badge (continuous param).
	idx := -1
	for i, badge := range dv.instEditorStepBadges {
		if badge != nil {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("no continuous control with a resolution ladder")
	}
	r := synthResolutionBinding{dv: dv, idx: idx, instID: instID}
	if !r.available() {
		t.Fatal("resolution lane should be available for a badged control")
	}
	start := dv.instEditorStepBadges[idx].Index()
	// +1 detent = coarser (higher index). Guard against already-max.
	if r.canStep(+1) {
		if !r.stepBy(+1) {
			t.Fatal("stepBy(+1) should move the rung")
		}
		if dv.instEditorStepBadges[idx].Index() != start+1 {
			t.Fatalf("rung should advance by one: start=%d now=%d", start, dv.instEditorStepBadges[idx].Index())
		}
	}
}

func TestSynthResolutionBinding_UnavailableForDiscrete(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	// A nil step badge = discrete param = no resolution lane.
	for i, badge := range dv.instEditorStepBadges {
		if badge == nil {
			r := synthResolutionBinding{dv: dv, idx: i, instID: dv.synthTabActiveInstrument()}
			if r.available() {
				t.Fatalf("idx %d has nil badge but reports available", i)
			}
			return
		}
	}
	t.Skip("no discrete control present")
}
