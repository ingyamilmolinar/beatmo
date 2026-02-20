//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestEQToggleZoneSyncsDrumViewState verifies that clicking the EQ toggle
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

	// Both should start in the same state.
	if dv.eqWaveformMode != dv.eqPanelZone.WaveformMode() {
		t.Fatalf("initial desync: dv.eqWaveformMode=%v zone=%v",
			dv.eqWaveformMode, dv.eqPanelZone.WaveformMode())
	}

	initialMode := dv.eqWaveformMode

	// Find the toggle button rect via the zone.
	toggleRect := dv.eqPanelZone.eqToggleBtn.Rect()
	if toggleRect.Empty() {
		t.Skip("toggle button has empty rect")
	}
	cx := (toggleRect.Min.X + toggleRect.Max.X) / 2
	cy := (toggleRect.Min.Y + toggleRect.Max.Y) / 2

	// Simulate click on the toggle button: press frame, then release frame.
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

	// The zone's waveformMode should have toggled.
	if dv.eqPanelZone.WaveformMode() == initialMode {
		t.Fatal("zone waveformMode did not toggle after button click")
	}

	// DrumView's field should be synced.
	if dv.eqWaveformMode != dv.eqPanelZone.WaveformMode() {
		t.Fatalf("desync after toggle: dv.eqWaveformMode=%v zone=%v",
			dv.eqWaveformMode, dv.eqPanelZone.WaveformMode())
	}
}

// TestEQToggleRoundTrip toggles on and off, verifying sync both ways.
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

	toggleRect := dv.eqPanelZone.eqToggleBtn.Rect()
	if toggleRect.Empty() {
		t.Skip("toggle button has empty rect")
	}
	cx := (toggleRect.Min.X + toggleRect.Max.X) / 2
	cy := (toggleRect.Min.Y + toggleRect.Max.Y) / 2

	clickToggle := func() {
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

	initial := dv.eqWaveformMode

	// Toggle on.
	clickToggle()
	if dv.eqWaveformMode == initial {
		t.Fatal("first toggle did not change eqWaveformMode")
	}
	if dv.eqWaveformMode != dv.eqPanelZone.WaveformMode() {
		t.Fatal("desync after first toggle")
	}

	// Toggle off (back to initial).
	clickToggle()
	if dv.eqWaveformMode != initial {
		t.Fatalf("second toggle should restore initial=%v, got %v", initial, dv.eqWaveformMode)
	}
	if dv.eqWaveformMode != dv.eqPanelZone.WaveformMode() {
		t.Fatal("desync after second toggle")
	}
}
