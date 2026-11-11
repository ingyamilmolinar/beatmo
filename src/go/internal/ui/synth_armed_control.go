package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synth_armed_control.go — which control is ARMED for inline barrel editing.
//
// At most one control is armed per instrument. Arming selects the control
// (focus graph) and locks any previously-armed one. Locking removes the entry.
// The ArmedControlEditor (rebuilt on arm) owns the kinetic barrel state.

func (dv *DrumView) synthControlArmed(instID instrument.ID) (int, bool) {
	if dv.instEditorArmedControl == nil || instID == "" {
		return -1, false
	}
	idx, ok := dv.instEditorArmedControl[instID]
	return idx, ok
}

func (dv *DrumView) armSynthControl(instID instrument.ID, idx int) {
	if instID == "" || idx < 0 {
		return
	}
	if dv.instEditorArmedControl == nil {
		dv.instEditorArmedControl = map[instrument.ID]int{}
	}
	dv.instEditorArmedControl[instID] = idx
	dv.setSynthSelectedControl(instID, idx)
	// Fresh editor bound to the new index; kinetic state starts at rest.
	dv.instEditorArmedEditor = newSynthArmedControlEditor(dv, instID, idx)
	// Drive the grow animation toward the 2×2 armed footprint.
	dv.setArmGrowSubject(armedScopeSynth, instID, idx, 1)
}

func (dv *DrumView) lockSynthControl(instID instrument.ID) {
	// Aim the grow animation back to rest for the control being locked; the
	// subject is retained by setArmGrowSubject/advanceArmGrow until it settles, so
	// the card visibly shrinks and neighbours slide home after it is unarmed.
	if dv.instEditorArmedControl != nil {
		if idx, ok := dv.instEditorArmedControl[instID]; ok {
			dv.setArmGrowSubject(armedScopeSynth, instID, idx, 0)
		}
		delete(dv.instEditorArmedControl, instID)
	}
	dv.instEditorArmedEditor = nil
}

// armedControlActive reports whether the ACTIVE audio-panel tab has a control
// armed for inline barrel editing. Nil-safe. Used by KeyboardClaimed (so armed
// arrow keys don't also pan the camera) and by handleEscape's armed rung (so Esc
// locks the control instead of falling through to Stop playback). Keeping ONE
// predicate across both tabs is what guarantees they get the same keyboard
// semantics.
func (dv *DrumView) armedControlActive() bool {
	if dv == nil || dv.eqPanelZone == nil {
		return false
	}
	switch dv.eqPanelZone.ActiveTab() {
	case TabSynth:
		if len(dv.instEditorArmedControl) == 0 || dv.instEditorArmedEditor == nil {
			return false
		}
		_, armed := dv.synthControlArmed(dv.synthTabActiveInstrument())
		return armed
	}
	// The TabSampler branch is added when the Sampler adopts the armed model
	// (sampler_armed_control.go).
	return false
}

func (dv *DrumView) toggleSynthControlArm(instID instrument.ID, idx int) {
	if cur, armed := dv.synthControlArmed(instID); armed && cur == idx {
		dv.lockSynthControl(instID)
		return
	}
	dv.armSynthControl(instID, idx)
}

// ── keyboard editing (KeyboardKeyClaimant, see keyboard_key_claim.go) ────────

// armedControlKeyClaimant returns the armed inline control as a PARTIAL keyboard
// claimant, or nil when none is armed on the active tab. It owns the arrows and
// nothing else, so Space still plays and [ ] still zoom while a control is
// armed.
func (dv *DrumView) armedControlKeyClaimant() KeyboardKeyClaimant {
	if dv == nil || !dv.armedControlActive() || dv.instEditorArmedEditor == nil {
		return nil
	}
	return armedControlKeys{dv: dv}
}

// armedControlKeys adapts the armed editor to KeyboardKeyClaimant. It resolves
// dv.instEditorArmedEditor on every call rather than caching it, so a re-arm
// (which rebuilds the editor) between claim and handle cannot act on a stale one.
type armedControlKeys struct{ dv *DrumView }

// ClaimedKeys: Up/Down step the value; Left/Right step the resolution, but only
// when this param has a resolution lane (a discrete param gets a value lane only
// — see newSynthArmedControlEditor).
func (a armedControlKeys) ClaimedKeys() []ebiten.Key {
	e := a.editor()
	if e == nil {
		return nil
	}
	if e.HasResolutionLane() {
		return valueAndResolutionStepKeys()
	}
	return valueStepKeys()
}

// HandleClaimedKeys: one key acts per frame (the switch), so the arrow-key alias
// spellings real Ebiten collapses cannot double-fire.
func (a armedControlKeys) HandleClaimedKeys() bool {
	e := a.editor()
	if e == nil {
		return false
	}
	switch {
	case anyKeyJustPressed(keysStepUp...):
		return e.StepValue(+1)
	case anyKeyJustPressed(keysStepDown...):
		return e.StepValue(-1)
	case anyKeyJustPressed(keysStepRight...):
		return e.StepResolution(+1)
	case anyKeyJustPressed(keysStepLeft...):
		return e.StepResolution(-1)
	}
	return false
}

func (a armedControlKeys) editor() *ArmedControlEditor {
	if a.dv == nil || !a.dv.armedControlActive() {
		return nil
	}
	return a.dv.instEditorArmedEditor
}
