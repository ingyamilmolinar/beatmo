//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Disabled under Master so the user cannot ENTER the tab, but once active (opened programmatically, showing row 0)
// the pill reads active, not blocked.
func TestSynthPillNotDisabledWhileSynthTabActive(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	z := g.drum.eqPanelZone

	z.SetActiveChannel(channel.Master)
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
