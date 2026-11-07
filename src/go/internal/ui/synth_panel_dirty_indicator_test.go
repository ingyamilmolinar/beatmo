//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// False with an empty overlay, true after an edit, false again after Reset.
func TestSynthSaveDirty_FollowsOverlay(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	inst := env.instID

	// Fresh state — no overrides → not dirty.
	if dv.synthSaveDirty(inst) {
		t.Errorf("synthSaveDirty should be false on fresh recipe with no overrides")
	}
	// Edit one param → dirty.
	audio.SetInstrumentParam(inst, "amp_decay", 1.7)
	if !dv.synthSaveDirty(inst) {
		t.Errorf("synthSaveDirty should be true after SetInstrumentParam(decay,1.7)")
	}
	// Reset clears overlay → not dirty.
	audio.ResetInstrumentParams(inst)
	if dv.synthSaveDirty(inst) {
		t.Errorf("synthSaveDirty should be false after ResetInstrumentParams")
	}
}

// Primary only when dirty, so the user sees there is something to save.
func TestSynthSaveButton_SpecTogglesByDirtyState(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	inst := env.instID

	saveBtn := findFooterButton(t, dv, synthSaveAsButtonTag)
	if saveBtn.SpecID == ComponentButtonPrimary {
		t.Errorf("clean state: Save SpecID = ComponentButtonPrimary, want Secondary (no unsaved changes)")
	}

	audio.SetInstrumentParam(inst, "amp_decay", 1.7)
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	// Re-layout so dirty-driven spec is recomputed.
	dv.eqPanelZone.Invalidate()
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())

	saveBtn = findFooterButton(t, dv, synthSaveAsButtonTag)
	if saveBtn.SpecID != ComponentButtonPrimary {
		t.Errorf("dirty state: Save SpecID = %v, want ComponentButtonPrimary", saveBtn.SpecID)
	}
}
