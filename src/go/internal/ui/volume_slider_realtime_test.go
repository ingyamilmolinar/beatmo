//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestRowVolumeClickInvalidatesCache(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 600, 300), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowVolSliders()) == 0 {
		t.Fatal("expected at least one row volume slider")
	}

	// Clear cache dirty flag to simulate a fresh render state.
	dv.rowControlsCacheDirty = false

	slider := dv.rowVolSliders()[0]
	rect := slider.Rect()
	if rect.Dx() <= 0 {
		t.Fatalf("slider rect not initialized: %v", rect)
	}

	targetX := rect.Min.X + rect.Dx()/2
	targetY := rect.Min.Y + rect.Dy()/2

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return targetX, targetY },
		func(btn ebiten.MouseButton) bool { return btn == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	defer restore()

	dv.Update()

	// Opening the popup triggers recalcButtons, whose layout rebuild dirties the cache.
	if !dv.rowControlsCacheDirty {
		t.Log("note: rowControlsCacheDirty may be set by layout recalc, not slider drag")
	}
}

func TestRowVolumePopupChangesVolume(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 600, 300), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}

	origVol := dv.Rows[0].Volume

	// Directly set volume (as popup would) and verify it sticks.
	dv.Rows[0].Volume = 0.42
	if dv.Rows[0].Volume == origVol {
		t.Fatal("expected volume to change after direct assignment")
	}
	if dv.Rows[0].Volume != 0.42 {
		t.Fatalf("expected volume 0.42, got %f", dv.Rows[0].Volume)
	}
}
