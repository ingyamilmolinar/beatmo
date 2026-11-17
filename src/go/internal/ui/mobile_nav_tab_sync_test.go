//go:build test

package ui

import "testing"

// Programmatic audio-tab switches (scene catalog, JS exports, keyboard)
// must keep the mobile bottom-nav segmented control in sync: the nav is a
// DERIVED view of the panel tab state, not a second source of truth.
// Regression: eq_tab_wave/spectrum/levels mobile screenshots showed Wave/
// Spectrum/Levels content while the nav still highlighted "EQ".
func TestMobileBottomNavTracksProgrammaticTabSwitch(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	dv := g.drum

	dv.SetMobileEQMode(true)
	if dv.currentViewMode != viewModeEQ {
		t.Fatalf("precondition: audio view should start at EQ, got %v", dv.currentViewMode)
	}

	dv.eqPanelZone.SetActiveTab(TabWave)

	if dv.currentViewMode != viewModeWave {
		t.Fatalf("currentViewMode = %v after SetActiveTab(TabWave), want viewModeWave", dv.currentViewMode)
	}
	want := segmentIndexForViewMode(viewModeWave)
	if got := dv.viewSwitchSegmented.Active(); got != want {
		t.Fatalf("bottom-nav active segment = %d, want %d (Wave)", got, want)
	}
}

// A background tab change while Pads owns the mobile screen must NOT
// hijack the user into the audio view.
func TestMobileBottomNavPadsNotHijackedByTabSwitch(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	dv := g.drum

	if dv.currentViewMode != viewModeRows {
		t.Fatalf("precondition: mobile boots in Pads, got %v", dv.currentViewMode)
	}

	dv.eqPanelZone.SetActiveTab(TabSpectrum)

	if dv.currentViewMode != viewModeRows {
		t.Fatalf("Pads hijacked: currentViewMode = %v, want viewModeRows", dv.currentViewMode)
	}
	if got := dv.viewSwitchSegmented.Active(); got != segmentIndexForViewMode(viewModeRows) {
		t.Fatalf("bottom-nav active segment = %d, want Pads", got)
	}
}
