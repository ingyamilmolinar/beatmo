//go:build test

package ui

import "testing"

// The nav is a derived view of the panel tab, never a second source of truth, so programmatic switches move it
// too.
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

// A background tab change must not pull the user out of Pads.
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
