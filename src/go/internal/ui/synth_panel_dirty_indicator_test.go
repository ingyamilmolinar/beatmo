//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSynthSaveDirty_FollowsOverlay — synthSaveDirty(instID) must report
// false when the per-instrument overlay is empty and true after any param
// edit, returning to false after Reset.
func TestSynthSaveDirty_FollowsOverlay(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	inst := env.instID

	// Fresh state — no overrides → not dirty.
	if dv.synthSaveDirty(inst) {
		t.Errorf("synthSaveDirty should be false on fresh recipe with no overrides")
	}
	// Edit one param → dirty.
	audio.SetInstrumentParam(inst, "decay", 1.7)
	if !dv.synthSaveDirty(inst) {
		t.Errorf("synthSaveDirty should be true after SetInstrumentParam(decay,1.7)")
	}
	// Reset clears overlay → not dirty.
	audio.ResetInstrumentParams(inst)
	if dv.synthSaveDirty(inst) {
		t.Errorf("synthSaveDirty should be false after ResetInstrumentParams")
	}
}

// TestSynthSaveButton_SpecTogglesByDirtyState — when synth params are clean,
// the Save button's spec must NOT be ComponentButtonPrimary (it's a no-op
// affordance). When dirty, the spec must be ComponentButtonPrimary so the
// user gets a visual signal that Save will do something.
func TestSynthSaveButton_SpecTogglesByDirtyState(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	inst := env.instID

	saveBtn := findFooterButton(t, dv, synthSaveButtonTag)
	if saveBtn.SpecID == ComponentButtonPrimary {
		t.Errorf("clean state: Save SpecID = ComponentButtonPrimary, want Secondary (no unsaved changes)")
	}

	audio.SetInstrumentParam(inst, "decay", 1.7)
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	// Re-layout so dirty-driven spec is recomputed.
	dv.eqPanelZone.Invalidate()
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())

	saveBtn = findFooterButton(t, dv, synthSaveButtonTag)
	if saveBtn.SpecID != ComponentButtonPrimary {
		t.Errorf("dirty state: Save SpecID = %v, want ComponentButtonPrimary", saveBtn.SpecID)
	}
}
