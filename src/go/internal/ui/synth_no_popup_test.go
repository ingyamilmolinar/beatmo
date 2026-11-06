//go:build test

package ui

import "testing"

// TestSynthTapDoesNotOpenWheelPopup guards Task 13: the synth barrel/wheel
// popup was retired from the synth control path (Task 9 replaced tap→popup
// with tap→arm). A tap-then-release on a synth control must ARM it inline,
// never open a modal wheel popup. The synthWheelPopup field itself no longer
// exists on DrumView — its removal is the strongest form of this guarantee —
// so this test only needs to assert the arm side of the contract.
func TestSynthTapDoesNotOpenWheelPopup(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	expandSynthPanelForTest(t, g)
	inst := dv.synthTabActiveInstrument()
	sec := dv.synthSelectedSection()
	if inst == "" || sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("expected an active synth instrument with at least one visible knob")
	}
	idx := sec.knobIdxs[0]
	selectSectionForKnobIdx(t, g, inst, idx)
	k := dv.instEditorControls[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatal("target knob not laid out")
	}

	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: inst}
	h.OnPress(r.Min.X+2, r.Min.Y+2)
	h.OnRelease(r.Min.X+2, r.Min.Y+2)

	if _, armed := dv.synthControlArmed(inst); !armed {
		t.Fatal("synth tap should arm the control inline")
	}
}
