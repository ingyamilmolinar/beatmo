//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestDesktopMasterVolIconOpensPopup verifies that clicking the master volume
// icon on desktop fires the OnMasterVolClick callback.
func TestDesktopMasterVolIconOpensPopup(t *testing.T) {
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

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	volIconArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-vol-icon")
	if volIconArea == nil {
		t.Fatal("expected 'transport-vol-icon' hit area on desktop")
	}

	mx = (volIconArea.Rect.Min.X + volIconArea.Rect.Max.X) / 2
	my = (volIconArea.Rect.Min.Y + volIconArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.volClicks == 0 {
		t.Error("expected OnMasterVolClick callback after clicking volume icon on desktop")
	}
}

// TestDesktopMasterVolNoInlineSlider verifies that no "transport-vol-slider"
// hit area is registered on desktop (popup mode only).
func TestDesktopMasterVolNoInlineSlider(t *testing.T) {
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	volSliderArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-vol-slider")
	if volSliderArea != nil {
		t.Error("desktop should NOT have 'transport-vol-slider' hit area — popup mode only")
	}
}

// TestDesktopMasterVolIconHitAreaExists verifies that the "transport-vol-icon"
// hit area exists on desktop.
func TestDesktopMasterVolIconHitAreaExists(t *testing.T) {
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	volIconArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-vol-icon")
	if volIconArea == nil {
		t.Fatal("expected 'transport-vol-icon' hit area on desktop")
	}
}

// TestDesktopMasterVolPopupDrag verifies that dragging inside the master
// volume popup changes audio.MainVolume().
func TestDesktopMasterVolPopupDrag(t *testing.T) {
	withDefaultAudio(t)
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.mainVolIconRect.Empty() {
		dv.mainVolIconRect = image.Rect(100, 100, 130, 130)
	}
	if dv.mainVolSlider() == nil {
		dv.transportZone.mainVolSlider = NewSlider(1.0)
	}
	audio.SetMainVolume(1.0)

	dv.openMasterVolumePopup()
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("popup should be open")
	}

	r := dv.masterVolPopup.Rect()
	mx := r.Min.X + r.Dx()/2
	// Drag to the bottom of the track → volume decreases toward 0.
	dv.masterVolPopup.HandleInput(mx, r.Max.Y-9, true)
	dv.masterVolPopup.HandleInput(mx, r.Max.Y-9, false)

	if audio.MainVolume() >= 1.0 {
		t.Fatalf("expected volume to decrease, got %f", audio.MainVolume())
	}
}

// TestDesktopExportButtonNotOverlapped verifies that the export, import, and
// upload button rects do not overlap the master volume icon area.
func TestDesktopExportButtonNotOverlapped(t *testing.T) {
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	volIcon := z.MainVolIconRect()
	if volIcon.Empty() {
		t.Fatal("mainVolIconRect is empty after layout")
	}

	for _, tag := range []string{"transport-upload", "transport-import", "transport-export"} {
		area := findHitAreaByTagPrefix(z.HitAreas(), tag)
		if area == nil {
			continue
		}
		overlap := area.Rect.Intersect(volIcon)
		if !overlap.Empty() {
			t.Errorf("%s rect %v overlaps master vol icon %v (overlap: %v)", tag, area.Rect, volIcon, overlap)
		}
	}
}

// TestDesktopMasterVolPopupCloseOnOutsideClick verifies that pressing outside
// the popup does not consume input (allowing dismissal).
func TestDesktopMasterVolPopupCloseOnOutsideClick(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.mainVolIconRect.Empty() {
		dv.mainVolIconRect = image.Rect(100, 100, 130, 130)
	}
	if dv.mainVolSlider() == nil {
		dv.transportZone.mainVolSlider = NewSlider(1.0)
	}

	dv.openMasterVolumePopup()
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("popup should be open")
	}

	// Press outside: should not consume input.
	consumed := dv.masterVolPopup.HandleInput(0, 0, true)
	if consumed {
		t.Fatal("pressing outside popup should not consume input")
	}
}
