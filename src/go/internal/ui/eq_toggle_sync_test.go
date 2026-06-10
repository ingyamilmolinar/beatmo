//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestEQToggleZoneSyncsDrumViewState verifies that clicking the Wave tab
// button in the zone tree syncs waveformMode to DrumView.eqWaveformMode.
func TestEQToggleZoneSyncsDrumViewState(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.eqPanelZone == nil {
		t.Skip("eqPanelZone not initialized")
	}
	if dv.tree == nil {
		t.Skip("tree not initialized")
	}

	// Both should start in the same state (EQ mode, not waveform).
	if dv.eqWaveformMode != (dv.eqPanelZone.ActiveTab() == TabWave) {
		t.Fatalf("initial desync: dv.eqWaveformMode=%v zone=%v",
			dv.eqWaveformMode, (dv.eqPanelZone.ActiveTab() == TabWave))
	}

	initialMode := dv.eqWaveformMode

	// Click the Wave tab button (index 1 since AllPanelTabs = [EQ, Wave, Spectrum, Meters]).
	waveBtn := dv.eqPanelZone.stickyBar.TabBtn(1)
	if waveBtn == nil {
		t.Skip("wave tab button is nil")
	}
	waveRect := waveBtn.Rect()
	if waveRect.Empty() {
		t.Skip("wave tab button has empty rect")
	}
	cx := (waveRect.Min.X + waveRect.Max.X) / 2
	cy := (waveRect.Min.Y + waveRect.Max.Y) / 2

	// Simulate click on the wave tab button: press frame, then release frame.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	// Release frame.
	restore()
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// The zone's waveformMode should have toggled (from EQ to Wave).
	if (dv.eqPanelZone.ActiveTab() == TabWave) == initialMode {
		t.Fatal("zone waveformMode did not change after clicking Wave tab")
	}

	// DrumView's field should be synced.
	if dv.eqWaveformMode != (dv.eqPanelZone.ActiveTab() == TabWave) {
		t.Fatalf("desync after toggle: dv.eqWaveformMode=%v zone=%v",
			dv.eqWaveformMode, (dv.eqPanelZone.ActiveTab() == TabWave))
	}
}

// TestEQToggleRoundTrip switches to Wave then back to EQ, verifying sync both ways.
func TestEQToggleRoundTrip(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.eqPanelZone == nil {
		t.Skip("eqPanelZone not initialized")
	}
	if dv.tree == nil {
		t.Skip("tree not initialized")
	}

	clickBtn := func(btn *Button) {
		r := btn.Rect()
		cx := (r.Min.X + r.Max.X) / 2
		cy := (r.Min.Y + r.Max.Y) / 2
		restore := SetInputForTest(
			func() (int, int) { return cx, cy },
			func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 800, 600 },
		)
		dv.Update()
		restore()
		restore = SetInputForTest(
			func() (int, int) { return cx, cy },
			func(b ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 800, 600 },
		)
		dv.Update()
		restore()
	}

	eqBtn := dv.eqPanelZone.stickyBar.TabBtn(0)
	waveBtn := dv.eqPanelZone.stickyBar.TabBtn(1)
	if eqBtn.Rect().Empty() || waveBtn.Rect().Empty() {
		t.Skip("tab button rects are empty")
	}

	initial := dv.eqWaveformMode

	// Switch to Wave.
	clickBtn(waveBtn)
	if dv.eqWaveformMode == initial {
		t.Fatal("clicking Wave tab did not change eqWaveformMode")
	}
	if dv.eqWaveformMode != (dv.eqPanelZone.ActiveTab() == TabWave) {
		t.Fatal("desync after switching to Wave")
	}

	// Switch back to EQ.
	clickBtn(eqBtn)
	if dv.eqWaveformMode != initial {
		t.Fatalf("clicking EQ tab should restore initial=%v, got %v", initial, dv.eqWaveformMode)
	}
	if dv.eqWaveformMode != (dv.eqPanelZone.ActiveTab() == TabWave) {
		t.Fatal("desync after switching back to EQ")
	}
}
