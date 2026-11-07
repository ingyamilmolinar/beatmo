package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// At most one control is armed per instrument; arming locks any previous one. The ArmedControlEditor owns
// the kinetic barrel state.

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
	// The subject is retained until the shrink settles, so the card visibly shrinks after it is unarmed.
	if dv.instEditorArmedControl != nil {
		if idx, ok := dv.instEditorArmedControl[instID]; ok {
			dv.setArmGrowSubject(armedScopeSynth, instID, idx, 0)
		}
		delete(dv.instEditorArmedControl, instID)
	}
	dv.instEditorArmedEditor = nil
}

// ONE predicate across both tabs, read by KeyboardClaimed and the Esc rung, so the tabs share keyboard semantics.
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

// A PARTIAL claimant owning the arrows only, so Space still plays and [ ] still zoom.
func (dv *DrumView) armedControlKeyClaimant() KeyboardKeyClaimant {
	if dv == nil || !dv.armedControlActive() || dv.instEditorArmedEditor == nil {
		return nil
	}
	return armedControlKeys{dv: dv}
}

// Resolves the editor on every call: a re-arm between claim and handle rebuilds it.
type armedControlKeys struct{ dv *DrumView }

// Left/Right only when the param has a resolution lane; a discrete param has none.
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

// One key per frame (the switch), so the arrow spellings real Ebiten aliases cannot double-fire.
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
