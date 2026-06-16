//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestVolumePopupRailUsesRowColor pins that the per-row volume slider popup
// paints its rail in the row's instrument color — the SAME color the grid nodes
// use (DrumRow.Color) — not the cyan genColorPrimary accent. The popup's rail
// fill is the widest filled rounded rect narrower than the full rail, so we look
// for a filled rounded rect carrying the row's color and assert it is NOT the
// cyan fallback.
func TestVolumePopupRailUsesRowColor(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect — cannot test popup rail")
	}

	// A distinctly non-cyan instrument color on row 0, and a non-zero level so
	// the rail draws a fill segment.
	rowCol := color.RGBA{R: 220, G: 40, B: 90, A: 255}
	dv.Rows[0].Color = rowCol
	dv.Rows[0].Volume = 0.8

	dv.openVolumePopup(0)
	dst := ebiten.NewImage(800, 600)
	calls := captureRoundedRectCalls(t, func() { dv.volPopup.Draw(dst) })

	wr, wg, wb, _ := rgba8(rowCol)
	cr, cg, cb, _ := rgba8(genColorPrimary)
	foundRowColor := false
	for _, c := range calls {
		if !c.Filled {
			continue
		}
		gr, gg, gb, _ := rgba8(c.Color)
		if gr == cr && gg == cg && gb == cb {
			t.Errorf("volume popup rail drew the cyan genColorPrimary fallback (%d,%d,%d) instead of the row's instrument color", cr, cg, cb)
		}
		if gr == wr && gg == wg && gb == wb {
			foundRowColor = true
		}
	}
	if !foundRowColor {
		t.Errorf("volume popup rail never drew the row's instrument color (%d,%d,%d); rail must match the grid nodes", wr, wg, wb)
	}
}

// TestRowColorSwitchUpdatesVolumeIcon pins that switching a row's instrument
// color repaints the per-row volume control (speaker icon + level indicator) in
// the NEW color on the next frame. The row controls are cached into an offscreen
// image gated by controlsCacheValid(); before the fix, SetRowColor* marked only
// the grid step-cell cache dirty, never the controls cache, so the cached
// speaker icon kept the STALE color until some unrelated state change happened
// to rebuild it. The popup (drawn uncached) updated, but the in-row icon did not.
func TestRowColorSwitchUpdatesVolumeIcon(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect — cannot exercise controls cache")
	}
	dv.Rows[0].Volume = 0.8

	// First draw builds the controls cache with the original color.
	img := ebiten.NewImage(800, 600)
	dv.rowRackZone.Draw(img)
	if !dv.rowRackZone.controlsCacheValid() {
		t.Fatal("precondition: controls cache should be valid after the initial draw")
	}

	// Switch row 0 to a distinct, non-default color.
	newCol := color.RGBA{R: 7, G: 222, B: 131, A: 255}
	dv.SetRowColorManual(0, newCol)

	// Contract: the color switch must invalidate the controls cache so the
	// cached volume icon is rebuilt with the new color next frame.
	if dv.rowRackZone.controlsCacheValid() {
		t.Error("switching a row's color must invalidate the row-rack controls cache so the volume icon repaints in the new color")
	}

	// End-to-end: the next draw must actually paint the new color (the volume
	// level-indicator fill draws a filled rounded rect in the row color). A stale
	// cache would blit the old image and draw nothing here.
	calls := captureRoundedRectCalls(t, func() { dv.rowRackZone.Draw(img) })
	wr, wg, wb, _ := rgba8(newCol)
	painted := false
	for _, c := range calls {
		if !c.Filled {
			continue
		}
		if gr, gg, gb, _ := rgba8(c.Color); gr == wr && gg == wg && gb == wb {
			painted = true
			break
		}
	}
	if !painted {
		t.Errorf("after color switch, no control repainted in the new color (%d,%d,%d) — the cached volume icon stayed stale", wr, wg, wb)
	}
}

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

// ---------- on-screen containment + proximity ----------

// TestSliderPopupBottomAnchorStaysOnScreen is the regression case for the
// "popup renders ~300px from its trigger over the EQ panel and clips below the
// screen so the LOW volume range is unreachable" bug. Opening from a
// bottom-edge anchor must keep the whole popup inside bounds.
func TestSliderPopupBottomAnchorStaysOnScreen(t *testing.T) {
	assertDefaultParityState(t)

	bounds := image.Rect(0, 0, 800, 600)
	// Anchor flush against the bottom edge.
	anchor := image.Rect(400, 580, 430, 600)

	v := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-bottom",
		GetValue: func() float64 { return v },
		SetValue: func(nv float64) { v = nv },
	})
	sp.SetTitle(func() string { return "Kick-1" })
	sp.Open(anchor, bounds, 44)

	r := sp.Rect()
	if r.Empty() {
		t.Fatal("popup rect empty after open")
	}
	if !r.In(bounds) {
		t.Fatalf("popup rect %v escapes bounds %v (LOW range unreachable)", r, bounds)
	}
	// The whole vertical track must be on-screen so the full range is reachable.
	if sp.trackBot() > bounds.Max.Y || sp.trackTop() < bounds.Min.Y {
		t.Fatalf("track [%d,%d] escapes bounds Y [%d,%d]",
			sp.trackTop(), sp.trackBot(), bounds.Min.Y, bounds.Max.Y)
	}
}

// TestSliderPopupSitsNearTrigger verifies the popup is positioned adjacent to
// its anchor (not detached ~300px away over the EQ panel).
func TestSliderPopupSitsNearTrigger(t *testing.T) {
	assertDefaultParityState(t)

	bounds := image.Rect(0, 0, 800, 600)
	anchor := image.Rect(400, 200, 430, 224)

	v := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-near",
		GetValue: func() float64 { return v },
		SetValue: func(nv float64) { v = nv },
	})
	sp.Open(anchor, bounds, 44)

	r := sp.Rect()
	// The popup must be adjacent to the anchor: its top/bottom edge within a
	// sane gap of the anchor's band, and horizontally overlapping the anchor.
	const maxGap = 24
	gapBelow := r.Min.Y - anchor.Max.Y // popup placed below the anchor
	gapAbove := anchor.Min.Y - r.Max.Y // or above
	near := (gapBelow >= 0 && gapBelow <= maxGap) || (gapAbove >= 0 && gapAbove <= maxGap)
	if !near {
		t.Fatalf("popup %v is not adjacent to anchor %v (gapBelow=%d gapAbove=%d)",
			r, anchor, gapBelow, gapAbove)
	}
	if r.Intersect(image.Rect(anchor.Min.X-60, r.Min.Y, anchor.Max.X+60, r.Max.Y)).Empty() {
		t.Fatalf("popup %v is horizontally detached from anchor %v", r, anchor)
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
