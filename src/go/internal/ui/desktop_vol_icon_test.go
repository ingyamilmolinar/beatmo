//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDesktopVolClickOpensPopup verifies that clicking the volume cell on
// desktop fires OnVolPopupOpen (not OnVolumeChange).
func TestDesktopVolClickOpensPopup(t *testing.T) {
	// Ensure desktop profile is active.
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	rows := makeTestRows(2)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	// Find the volume icon hit area (should exist on desktop now).
	volIconArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-vol-icon")
	if volIconArea == nil {
		t.Fatal("expected 'row-rack-vol-icon' hit area on desktop")
	}

	mx = (volIconArea.Rect.Min.X + volIconArea.Rect.Max.X) / 2
	my = (volIconArea.Rect.Min.Y + volIconArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.volPopupOpens) == 0 {
		t.Error("expected OnVolPopupOpen callback after clicking volume icon on desktop")
	}
	if len(log.volumeChanges) != 0 {
		t.Error("expected no OnVolumeChange callbacks — desktop should open popup, not inline change")
	}
}

// TestDesktopVolIconHitArea verifies that the "row-rack-vol-icon" hit area
// exists on desktop (not just mobile).
func TestDesktopVolIconHitArea(t *testing.T) {
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	volIconArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-vol-icon")
	if volIconArea == nil {
		t.Fatal("expected 'row-rack-vol-icon' hit area on desktop layout")
	}
}

// TestDesktopVolColumnWeight asserts the desktop volume column weight is
// narrower than the old slider weight (was 7, now should be smaller).
func TestDesktopVolColumnWeight(t *testing.T) {
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	weights := rowControlWeights()
	// Column index 3 is the volume column.
	if weights[3] >= 7 {
		t.Errorf("desktop volume column weight should be < 7 (icon+%%), got %.1f", weights[3])
	}
}
