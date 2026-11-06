package ui

// synth_armed_keys_test.go — Task 10: arrow/Esc keyboard editing while a synth
// control is ARMED, plus the keyboard-claim guard that suppresses grid arrow
// pan while armed (see keyboard_focus.go's KeyboardClaimed contract).

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

	// Drive the REAL key path: the armed control is a partial keyboard claimant
	// (keyboard_key_claim.go), so handleGlobalShortcuts — not synthTabUpdate — is
	// the single site that reads its arrows and masks the same press for the
	// grid's arrow-pan. Reading them in synthTabUpdate too would step twice.
	withStubbedKeys(nil, []ebiten.Key{ebiten.KeyArrowUp}, func() {
		g.handleGlobalShortcuts()
	})
	// Drain the kinetic animation the StepImmediate impulse kicked off so the
	// knob's Value has settled before we compare.
	for i := 0; i < 120 && dv.instEditorArmedEditor != nil && dv.instEditorArmedEditor.Tick(); i++ {
	}

	after := dv.instEditorControls[idx].Value
	if after <= before {
		t.Fatalf("ArrowUp should increase the armed value: before=%v after=%v", before, after)
	}

	if _, armed := dv.synthControlArmed(inst); !armed {
		t.Fatal("control should still be armed before Esc")
	}

	// Esc is owned by handleEscape (the SINGLE authority for the Esc key),
	// NOT synthTabUpdate — drive the real path here, exactly as Game.Update
	// would each frame. This is the regression guard for the Task 10
	// Critical bug: arming a control opens no portal/focus/mode, so before
	// the fix, handleEscape fell through its whole ladder to the Stop
	// fallback and reset playback as an unrequested side effect of locking
	// the armed control.
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
	// An armed control owns the ARROWS, not the whole keyboard: it is a PARTIAL
	// claimant, so Space still plays and [ ] still zoom while it is armed.
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
