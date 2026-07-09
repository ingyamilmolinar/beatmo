//go:build test

package ui

import "testing"

// The Synth/Sampler tab pills are disabled under the Master channel to stop
// the user ENTERING them — but once the tab IS active (programmatic open;
// content resolves the Rows[0] fallback), the pill must render as the
// active tab, not as a blocked grey pill under live per-instrument content
// (the desktop eq_tab_synth screenshot leak, 2026-07-04 critique).
func TestSynthPillNotDisabledWhileSynthTabActive(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	z := g.drum.eqPanelZone

	z.SetActiveChannel("main")
	if !z.synthTabDisabled() {
		t.Fatal("precondition: Synth pill should be blocked on Master while elsewhere")
	}

	z.SetActiveTab(TabSynth)
	if z.synthTabDisabled() {
		t.Fatal("Synth tab is ACTIVE — its pill must not render as blocked")
	}

	z.SetActiveTab(TabSampler)
	if z.samplerTabDisabled() {
		t.Fatal("Sampler tab is ACTIVE — its pill must not render as blocked")
	}
	// And the sibling that is NOT active stays blocked.
	if !z.synthTabDisabled() {
		t.Fatal("Synth pill should stay blocked on Master while the Sampler tab is active")
	}
}
