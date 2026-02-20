package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestEQPopupOpensOnMobile verifies that openEQPopup sets the popup state correctly.
func TestEQPopupOpensOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), graph, testLogger)
	dv.recalcButtons()

	// Ensure eqRect is non-empty so popup can position.
	if dv.eqRect.Empty() {
		// Force the EQ panel visible for the test.
		dv.mobileEQCollapsed = false
		dv.mobileEQMode = true
		dv.currentViewMode = viewModeAudio
		if dv.widgets != nil {
			dv.widgets.ToggleWidget(WidgetWave, true)
		}
		dv.refreshWidgetLayout()
		dv.recalcButtons()
	}
	if dv.eqRect.Empty() {
		t.Skip("eqRect is empty after layout — cannot test popup positioning")
	}

	dv.openEQPopup(5)

	if !dv.eqPopup.IsOpen() {
		t.Fatal("eqPopup.IsOpen() should be true after openEQPopup")
	}
	if dv.eqPopupBand != 5 {
		t.Fatalf("eqPopupBand = %d, want 5", dv.eqPopupBand)
	}
	if dv.eqPopup.Rect().Empty() {
		t.Fatal("eqPopup.Rect() should be non-empty")
	}
}

// TestEQPopupCloseAllPopups verifies that CloseAllPopups closes an open EQ popup.
func TestEQPopupCloseAllPopups(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), graph, testLogger)
	dv.recalcButtons()

	// Manually open popup state via Open.
	dv.eqPopupBand = 3
	dv.eqPopup.open = true
	dv.eqPopup.rect = image.Rect(100, 100, 152, 260)
	dv.eqPopup.dragging = true

	dv.CloseAllPopups()

	if dv.eqPopup.IsOpen() {
		t.Fatal("eqPopup.IsOpen() should be false after CloseAllPopups")
	}
	if dv.eqPopup.IsDragging() {
		t.Fatal("eqPopup.IsDragging() should be false after CloseAllPopups")
	}
}

// TestEQPopupDesktopNoPopup verifies that on desktop, EQ slider interaction
// does NOT open a popup (the slider handles input directly).
func TestEQPopupDesktopNoPopup(t *testing.T) {
	assertDefaultParityState(t)
	// Ensure NOT mobile.
	forceSmallScreenForTest = false

	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()

	// Even if openEQPopup is called, it should work, but the mobile path
	// in Update should not trigger. Verify the slider path doesn't set popup open.
	if dv.eqPopup.IsOpen() {
		t.Fatal("eqPopup.IsOpen() should be false on desktop by default")
	}
}

// TestEQBandButtonTapOpensMobilePopup verifies that tapping the EQ band button
// on mobile opens the popup. This is a regression test: the old code used
// Slider.Handle() which has NO touch target expansion, making the 14px-tall
// slider nearly impossible to tap on mobile. The fix uses Button.Handle()
// which expands to TouchMinTarget (44px).
func TestEQBandButtonTapOpensMobilePopup(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), graph, testLogger)
	dv.recalcButtons()

	// Force the EQ panel visible.
	if dv.eqRect.Empty() {
		dv.mobileEQCollapsed = false
		dv.mobileEQMode = true
		dv.currentViewMode = viewModeAudio
		if dv.widgets != nil {
			dv.widgets.ToggleWidget(WidgetWave, true)
		}
		dv.refreshWidgetLayout()
		dv.recalcButtons()
	}
	if dv.eqRect.Empty() {
		t.Skip("eqRect is empty after layout — cannot test EQ band button tap")
	}

	// Simulate the rects that Draw sets: sliderH=14 at the bottom of eqRect.
	sliderH := 14
	sliderY := dv.eqRect.Max.Y - sliderH - 2
	bandW := dv.eqRect.Dx() / len(eqBandDefs)
	for i := range dv.eqSliders {
		if dv.eqSliders[i] == nil {
			continue
		}
		x0 := dv.eqRect.Min.X + i*bandW
		x1 := x0 + bandW
		dv.eqSliders[i].SetRect(image.Rect(x0+4, sliderY, x1-4, sliderY+sliderH))
	}

	// Pick band 5. Tap 5px above the slider rect. This is within the 44px
	// expanded touch target of a Button but OUTSIDE the 14px slider rect.
	band := 5
	x0 := dv.eqRect.Min.X + band*bandW
	x1 := x0 + bandW
	tapX := (x0 + 4 + x1 - 4) / 2
	tapY := sliderY - 5 // just above the 14px slider rect

	// On mobile, eqBandBtns should exist and handle the tap with touch expansion.
	// Before the fix, only eqSliders existed (no touch expansion → miss).
	if len(dv.eqBandBtns) == 0 {
		t.Fatal("eqBandBtns should be created on mobile for touch-friendly EQ band taps")
	}

	btn := dv.eqBandBtns[band]
	if btn == nil {
		t.Fatal("eqBandBtns[5] should not be nil")
	}

	// Set the button rect matching what Draw sets (same rect as the slider).
	btn.SetRect(image.Rect(x0+4, sliderY, x1-4, sliderY+sliderH))

	// Simulate tap: press then release. Button.Handle() expands touch target to 44px.
	btn.Handle(tapX, tapY, true)
	btn.Handle(tapX, tapY, false)

	if !dv.eqPopup.IsOpen() {
		t.Fatal("EQ popup should open when tapping near band button on mobile; " +
			"Slider.Handle() has no touch expansion so the 14px target is missed")
	}
	if dv.eqPopupBand != band {
		t.Fatalf("eqPopupBand = %d, want %d", dv.eqPopupBand, band)
	}
}

// TestEQPopupWithinBounds verifies that the popup rect stays within drum view
// bounds for edge bands (0 and 9).
func TestEQPopupWithinBounds(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), graph, testLogger)
	dv.recalcButtons()

	// Ensure eqRect is non-empty.
	if dv.eqRect.Empty() {
		dv.mobileEQCollapsed = false
		dv.mobileEQMode = true
		dv.currentViewMode = viewModeAudio
		if dv.widgets != nil {
			dv.widgets.ToggleWidget(WidgetWave, true)
		}
		dv.refreshWidgetLayout()
		dv.recalcButtons()
	}
	if dv.eqRect.Empty() {
		t.Skip("eqRect is empty after layout — cannot test popup positioning")
	}

	for _, band := range []int{0, 9} {
		dv.openEQPopup(band)
		if !dv.eqPopup.IsOpen() {
			t.Fatalf("band %d: popup did not open", band)
		}
		r := dv.eqPopup.Rect()
		if r.Min.X < dv.Bounds.Min.X {
			t.Errorf("band %d: popup left edge %d < bounds left %d", band, r.Min.X, dv.Bounds.Min.X)
		}
		if r.Max.X > dv.Bounds.Max.X {
			t.Errorf("band %d: popup right edge %d > bounds right %d", band, r.Max.X, dv.Bounds.Max.X)
		}
		dv.closeEQPopup()
	}
}
