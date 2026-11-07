package ui

// Arrow/Esc editing of an armed control, which claims the arrows so the grid does not pan.

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestArmedArrowKeysEditValueAndEsc(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	layoutSynthTab(t, g)

	inst := dv.synthTabActiveInstrument()
	idx := dv.synthSelectedSection().knobIdxs[0]
	dv.armSynthControl(inst, idx)

	before := dv.instEditorControls[idx].Value

	// The armed control is a partial claimant, so handleGlobalShortcuts is the one site that reads its arrows and
	// masks them for the grid; a second reader would step twice.
	withStubbedKeys(nil, []ebiten.Key{ebiten.KeyArrowUp}, func() {
		g.handleGlobalShortcuts()
	})
	// Drain the StepImmediate animation so the value has settled.
	for i := 0; i < 120 && dv.instEditorArmedEditor != nil && dv.instEditorArmedEditor.Tick(); i++ {
	}

	after := dv.instEditorControls[idx].Value
	if after <= before {
		t.Fatalf("ArrowUp should increase the armed value: before=%v after=%v", before, after)
	}

	if _, armed := dv.synthControlArmed(inst); !armed {
		t.Fatal("control should still be armed before Esc")
	}

	// Through handleEscape, the one Esc authority: arming opens no portal or mode, so its ladder must lock the control
	// rather than fall through to Stop.
	withStubbedKeys(nil, []ebiten.Key{ebiten.KeyEscape}, func() {
		g.handleEscape()
	})

	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("Esc should lock (disarm) the armed control")
	}
	if dv.StopPressed() {
		t.Fatal("Esc-to-lock-armed-control must not also trigger Stop (Task 10 Critical regression)")
	}
}

func TestArmedControlClaimsKeyboard(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	layoutSynthTab(t, g)

	inst := dv.synthTabActiveInstrument()
	idx := dv.synthSelectedSection().knobIdxs[0]

	if dv.armedControlActive() {
		t.Fatal("armedControlActive should be false before arming")
	}
	if len(dv.KeyClaimants(nil)) != 0 {
		t.Fatal("nothing should claim keys before arming (grid arrow-pan must still work)")
	}

	dv.armSynthControl(inst, idx)

	if !dv.armedControlActive() {
		t.Fatal("armedControlActive should be true while armed on the Synth tab")
	}
	// A partial claimant: Space still plays and [ ] still zoom.
	if dv.KeyboardClaimed() {
		t.Fatal("an armed control must NOT claim the whole keyboard — it declares only the keys it handles")
	}
	claimants := dv.KeyClaimants(nil)
	if len(claimants) != 1 {
		t.Fatalf("armed control should be the one partial claimant; got %d", len(claimants))
	}
	if len(claimants[0].ClaimedKeys()) == 0 {
		t.Fatal("an armed control must claim the arrow keys it handles")
	}

	dv.lockSynthControl(inst)

	if dv.armedControlActive() {
		t.Fatal("armedControlActive should be false again after lockSynthControl")
	}
	if len(dv.KeyClaimants(nil)) != 0 {
		t.Fatal("locking must release the arrow claim so grid arrow-pan resumes")
	}
}
