//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// ---------- per-row volume popup ----------

// TestVolumePopupOpenClose verifies that openVolumePopup sets the expected
// state and closeVolumePopup resets it.
func TestVolumePopupOpenClose(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row after NewDrumView")
	}
	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect after layout — cannot test popup positioning")
	}

	dv.openVolumePopup(0)

	if !dv.volPopup.IsOpen() {
		t.Fatal("volPopup.IsOpen() should be true after openVolumePopup")
	}
	if dv.volPopupRow != 0 {
		t.Fatalf("volPopupRow = %d, want 0", dv.volPopupRow)
	}
	if dv.volPopup.Rect().Empty() {
		t.Fatal("volPopup.Rect() should be non-empty after open")
	}

	dv.closeVolumePopup()

	if dv.volPopup.IsOpen() {
		t.Fatal("volPopup.IsOpen() should be false after closeVolumePopup")
	}
	if dv.volPopup.IsDragging() {
		t.Fatal("volPopup.IsDragging() should be false after closeVolumePopup")
	}
}

// TestVolumePopupBoundsCheck verifies that openVolumePopup is a no-op for
// out-of-range row indices.
func TestVolumePopupBoundsCheck(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	// Negative row index.
	dv.openVolumePopup(-1)
	if dv.volPopup.IsOpen() {
		t.Fatal("popup should not open for negative row index")
	}

	// Row index beyond length.
	dv.openVolumePopup(len(dv.Rows) + 10)
	if dv.volPopup.IsOpen() {
		t.Fatal("popup should not open for row index beyond len(Rows)")
	}
}

// TestVolumePopupInputDrag verifies that pressing inside the popup activates
// dragging and changes the row volume.
func TestVolumePopupInputDrag(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect — cannot test popup input")
	}

	dv.Rows[0].Volume = 0.5
	dv.openVolumePopup(0)

	r := dv.volPopup.Rect()
	// Press in the center of the popup.
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	consumed := dv.volPopup.HandleInput(mx, my, true)

	if !consumed {
		t.Fatal("expected HandleInput to return true for press inside popup")
	}
	if !dv.volPopup.IsDragging() {
		t.Fatal("volPopup.IsDragging() should be true after press inside popup")
	}
	// Volume should have changed from the original 0.5.
	if dv.Rows[0].Volume == 0.5 {
		t.Fatal("volume should have changed after drag interaction")
	}

	// Release.
	dv.volPopup.HandleInput(mx, my, false)
	if dv.volPopup.IsDragging() {
		t.Fatal("volPopup.IsDragging() should be false after release")
	}
}

// TestVolumePopupInputClamping verifies that dragging above the track top
// clamps to 1.0 and dragging below clamps to 0.0.
func TestVolumePopupInputClamping(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect — cannot test popup clamping")
	}

	dv.Rows[0].Volume = 0.5
	dv.openVolumePopup(0)

	r := dv.volPopup.Rect()
	mx := r.Min.X + r.Dx()/2

	// Start a drag inside the popup first (required to set dragging).
	dv.volPopup.HandleInput(mx, r.Min.Y+r.Dy()/2, true)
	if !dv.volPopup.IsDragging() {
		t.Fatal("expected IsDragging() after initial press inside popup")
	}

	// Now drag far above the popup → volume should clamp to 1.0.
	dv.volPopup.HandleInput(mx, r.Min.Y-100, true)
	if dv.Rows[0].Volume != 1.0 {
		t.Fatalf("volume = %f after dragging above top, want 1.0", dv.Rows[0].Volume)
	}

	// Drag far below the popup → volume should clamp to 0.0.
	dv.volPopup.HandleInput(mx, r.Max.Y+100, true)
	if dv.Rows[0].Volume != 0.0 {
		t.Fatalf("volume = %f after dragging below bottom, want 0.0", dv.Rows[0].Volume)
	}

	// Release.
	dv.volPopup.HandleInput(mx, r.Max.Y+100, false)
}

// TestVolumePopupInputOutside verifies that pressing outside the popup rect
// (and not already dragging) does not consume input.
func TestVolumePopupInputOutside(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect — cannot test popup input")
	}

	dv.openVolumePopup(0)

	// Press well outside the popup rect.
	consumed := dv.volPopup.HandleInput(0, 0, true)
	if consumed {
		t.Fatal("expected HandleInput to return false for press outside popup")
	}
}

// ---------- master volume popup ----------

// TestMasterVolumePopupOpenClose verifies that openMasterVolumePopup sets
// state correctly and closeMasterVolumePopup resets it.
func TestMasterVolumePopupOpenClose(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	// Ensure mainVolIconRect is populated; if not, set it manually.
	if dv.mainVolIconRect.Empty() {
		dv.mainVolIconRect = image.Rect(100, 100, 130, 130)
	}
	if dv.mainVolSlider() == nil {
		dv.transportZone.mainVolSlider = NewSlider(1.0)
	}

	dv.openMasterVolumePopup()

	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("masterVolPopup.IsOpen() should be true after openMasterVolumePopup")
	}
	if dv.masterVolPopup.Rect().Empty() {
		t.Fatal("masterVolPopup.Rect() should be non-empty after open")
	}

	dv.closeMasterVolumePopup()

	if dv.masterVolPopup.IsOpen() {
		t.Fatal("masterVolPopup.IsOpen() should be false after close")
	}
	if dv.masterVolPopup.IsDragging() {
		t.Fatal("masterVolPopup.IsDragging() should be false after close")
	}
}

// TestMasterVolumePopupInputDrag verifies that dragging inside the master
// volume popup changes the slider value and calls audio.SetMainVolume.
func TestMasterVolumePopupInputDrag(t *testing.T) {
	withDefaultAudio(t)

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

	r := dv.masterVolPopup.Rect()
	mx := r.Min.X + r.Dx()/2

	// Drag to the bottom of the track area → volume should decrease toward 0.
	consumed := dv.masterVolPopup.HandleInput(mx, r.Max.Y-9, true)
	if !consumed {
		t.Fatal("expected HandleInput to return true")
	}
	if !dv.masterVolPopup.IsDragging() {
		t.Fatal("masterVolPopup.IsDragging() should be true after drag")
	}

	vol := dv.mainVolSlider().Value
	if vol >= 1.0 {
		t.Fatalf("slider value should have decreased from 1.0, got %f", vol)
	}

	gotAudioVol := audio.MainVolume()
	if gotAudioVol >= 1.0 {
		t.Fatalf("audio.MainVolume() should have decreased from 1.0, got %f", gotAudioVol)
	}

	// Release.
	dv.masterVolPopup.HandleInput(mx, r.Max.Y-9, false)
	if dv.masterVolPopup.IsDragging() {
		t.Fatal("masterVolPopup.IsDragging() should be false after release")
	}
}

// ---------- overlay wrappers ----------

// TestVolumePopupOverlayInterface verifies that SliderPopupOverlay (volume)
// satisfies the expected Overlay behaviour.
func TestVolumePopupOverlayInterface(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	o := &SliderPopupOverlay{Popup: dv.volPopup}

	if o.ID() != "volume-popup" {
		t.Fatalf("ID() = %q, want %q", o.ID(), "volume-popup")
	}
	if o.ZIndex() != 225 {
		t.Fatalf("ZIndex() = %d, want 225", o.ZIndex())
	}

	// Initially closed.
	if o.IsOpen() {
		t.Fatal("IsOpen() should be false before opening popup")
	}

	// Open the popup (need valid slider rect).
	if len(dv.rowVolSliders()) > 0 && !dv.rowVolSliders()[0].Rect().Empty() {
		dv.openVolumePopup(0)
		if !o.IsOpen() {
			t.Fatal("IsOpen() should be true after openVolumePopup")
		}

		// InputBounds should match the popup rect.
		if o.InputBounds() != dv.volPopup.Rect() {
			t.Fatalf("InputBounds() = %v, want %v", o.InputBounds(), dv.volPopup.Rect())
		}

		// Capturing should be false until a drag starts.
		if o.Capturing() {
			t.Fatal("Capturing() should be false before any drag")
		}

		// Close via overlay.
		o.Close()
		if o.IsOpen() {
			t.Fatal("IsOpen() should be false after Close()")
		}
	}
}

// TestMasterVolumePopupOverlayInterface verifies that SliderPopupOverlay (master)
// satisfies the expected Overlay behaviour.
func TestMasterVolumePopupOverlayInterface(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.mainVolIconRect.Empty() {
		dv.mainVolIconRect = image.Rect(100, 100, 130, 130)
	}
	if dv.mainVolSlider() == nil {
		dv.transportZone.mainVolSlider = NewSlider(1.0)
	}

	o := &SliderPopupOverlay{Popup: dv.masterVolPopup}

	if o.ID() != "master-volume-popup" {
		t.Fatalf("ID() = %q, want %q", o.ID(), "master-volume-popup")
	}
	if o.ZIndex() != 226 {
		t.Fatalf("ZIndex() = %d, want 226", o.ZIndex())
	}

	if o.IsOpen() {
		t.Fatal("IsOpen() should be false before opening popup")
	}

	dv.openMasterVolumePopup()
	if !o.IsOpen() {
		t.Fatal("IsOpen() should be true after openMasterVolumePopup")
	}
	if o.InputBounds() != dv.masterVolPopup.Rect() {
		t.Fatalf("InputBounds() = %v, want %v", o.InputBounds(), dv.masterVolPopup.Rect())
	}
	if o.Capturing() {
		t.Fatal("Capturing() should be false before any drag")
	}

	o.Close()
	if o.IsOpen() {
		t.Fatal("IsOpen() should be false after Close()")
	}
}

// TestVolumePopupOverlayHandleInput verifies that the overlay HandleInput
// returns the correct InputResult for drag vs consume vs ignore.
func TestVolumePopupOverlayHandleInput(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect — cannot test overlay input")
	}

	o := &SliderPopupOverlay{Popup: dv.volPopup}

	// When popup is closed, input should be ignored.
	result := o.HandleInput(100, 100, true)
	if result != InputIgnored {
		t.Fatalf("HandleInput when closed = %v, want InputIgnored", result)
	}

	dv.openVolumePopup(0)
	r := dv.volPopup.Rect()

	// Press inside the popup rect should consume or capture.
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	result = o.HandleInput(mx, my, true)
	if result != InputCaptured {
		t.Fatalf("HandleInput inside popup = %v, want InputCaptured", result)
	}

	// While dragging, HandleWheel should consume (prevent scroll-through).
	wResult := o.HandleWheel(mx, my, 3)
	if wResult != InputConsumed {
		t.Fatalf("HandleWheel = %v, want InputConsumed", wResult)
	}

	// Release.
	o.HandleInput(mx, my, false)
}

// TestMasterVolumePopupOverlayHandleInput verifies the master overlay's
// HandleInput returns appropriate results.
func TestMasterVolumePopupOverlayHandleInput(t *testing.T) {
	withDefaultAudio(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.mainVolIconRect.Empty() {
		dv.mainVolIconRect = image.Rect(100, 100, 130, 130)
	}
	if dv.mainVolSlider() == nil {
		dv.transportZone.mainVolSlider = NewSlider(1.0)
	}

	o := &SliderPopupOverlay{Popup: dv.masterVolPopup}

	// Closed: input ignored.
	result := o.HandleInput(100, 100, true)
	if result != InputIgnored {
		t.Fatalf("HandleInput when closed = %v, want InputIgnored", result)
	}

	dv.openMasterVolumePopup()
	r := dv.masterVolPopup.Rect()

	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	result = o.HandleInput(mx, my, true)
	if result != InputCaptured {
		t.Fatalf("HandleInput inside master popup = %v, want InputCaptured", result)
	}

	// HandleWheel should consume.
	wResult := o.HandleWheel(mx, my, -2)
	if wResult != InputConsumed {
		t.Fatalf("HandleWheel = %v, want InputConsumed", wResult)
	}

	// Release.
	o.HandleInput(mx, my, false)
}
