package ui

import "testing"

func TestSettingsGearOpensSettingsOverlay(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1200, 800)
	g.toggleSettingsOverlay()
	p := g.drum.portal()
	if p == nil || !p.Has(settingsOverlayID) {
		t.Fatal("toggleSettingsOverlay did not open the settings portal")
	}
	g.toggleSettingsOverlay()
	if p.Has(settingsOverlayID) {
		t.Fatal("second toggle did not close the settings portal")
	}
}

func TestGridHelpButtonUsesSettingsIcon(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.gridHelpBtn == nil {
		t.Skip("no grid help button on this profile")
	}
	if g.gridHelpBtn.Icon != string(IconSettings) {
		t.Fatalf("grid button icon=%q want %q", g.gridHelpBtn.Icon, string(IconSettings))
	}
}
