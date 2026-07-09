//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSliderHandleInputResult_PressStartsCapture verifies that pressing
// inside the slider starts a drag and returns InputCaptured.
func TestSliderHandleInputResult_PressStartsCapture(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0.5)
	s.SetRect(image.Rect(0, 0, 100, 20))

	r := s.HandleInputResult(50, 10, true)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured on press, got %d", r)
	}
	if !s.Capturing() {
		t.Fatal("slider should be capturing during drag")
	}
}

// TestSliderHandleInputResult_DragMaintainsCapture verifies that continued
// pressing during a drag returns InputCaptured and tracks the value.
func TestSliderHandleInputResult_DragMaintainsCapture(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0)
	s.SetRect(image.Rect(0, 0, 100, 20))

	// Start drag.
	s.HandleInputResult(10, 10, true)

	// Continue dragging outside bounds — should still capture.
	r := s.HandleInputResult(200, 50, true)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured during drag, got %d", r)
	}
	if !s.Capturing() {
		t.Fatal("should still be capturing")
	}
}

// TestSliderHandleInputResult_ReleaseReturnsConsumed verifies that releasing
// after a drag returns InputConsumed and clears capture.
func TestSliderHandleInputResult_ReleaseReturnsConsumed(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0)
	s.SetRect(image.Rect(0, 0, 100, 20))

	// Start drag.
	s.HandleInputResult(50, 10, true)

	// Release.
	r := s.HandleInputResult(50, 10, false)
	if r != InputConsumed {
		t.Fatalf("expected InputConsumed on release, got %d", r)
	}
	if s.Capturing() {
		t.Fatal("should not be capturing after release")
	}
}

// TestSliderHandleInputResult_MissReturnsIgnored verifies that pressing
// outside the slider returns InputIgnored.
func TestSliderHandleInputResult_MissReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0.5)
	s.SetRect(image.Rect(50, 50, 150, 70))

	r := s.HandleInputResult(0, 0, true)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored on miss, got %d", r)
	}
	if s.Capturing() {
		t.Fatal("should not be capturing on miss")
	}
}

// TestSliderHandleInputResult_ReleaseWithoutDragIgnored verifies that
// releasing without a prior drag returns InputIgnored.
func TestSliderHandleInputResult_ReleaseWithoutDragIgnored(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0.5)
	s.SetRect(image.Rect(0, 0, 100, 20))

	r := s.HandleInputResult(50, 10, false)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored on idle release, got %d", r)
	}
}

// TestSliderHandleInputResult_SuppressReturnsIgnored verifies that the
// suppress guard blocks slider input and clears on release.
func TestSliderHandleInputResult_SuppressReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0.5)
	s.SetRect(image.Rect(0, 0, 100, 20))

	r := s.HandleInputResult(50, 10, true)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored during suppress, got %d", r)
	}
	if s.Capturing() {
		t.Fatal("should not be capturing during suppress")
	}

	// Release clears suppress.
	r = s.HandleInputResult(50, 10, false)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored on release, got %d", r)
	}
	if suppressClicksUntilRelease {
		t.Fatal("suppress should be cleared after release")
	}
}

// TestSliderHandleInputResult_BackwardCompat verifies that Handle() returns
// the same booleans as before the refactor.
func TestSliderHandleInputResult_BackwardCompat(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0)
	s.SetRect(image.Rect(0, 0, 100, 20))

	// Press inside → consumed (starts drag)
	if s.HandleInputResult(50, 10, true) == InputIgnored {
		t.Fatal("Handle should consume press inside")
	}
	// Continue drag → consumed
	if s.HandleInputResult(80, 10, true) == InputIgnored {
		t.Fatal("Handle should consume during drag")
	}
	// Release → consumed (ends drag)
	if s.HandleInputResult(80, 10, false) == InputIgnored {
		t.Fatal("Handle should consume on drag release")
	}
	// Press outside → ignored
	if s.HandleInputResult(200, 200, true) != InputIgnored {
		t.Fatal("Handle should ignore press outside")
	}
}

// TestSliderHandleInputResult_ValueTracking verifies that HandleInputResult
// updates the slider value during drag (same as Handle).
func TestSliderHandleInputResult_ValueTracking(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0)
	s.SetRect(image.Rect(0, 0, 100, 10))

	// Press at right edge of track.
	s.HandleInputResult(s.TrackRect().Max.X-1, 5, true)
	if s.Value != 1.0 {
		t.Fatalf("expected value 1.0 at right edge, got %f", s.Value)
	}

	// Drag to left edge.
	s.HandleInputResult(s.TrackRect().Min.X, 5, true)
	if s.Value != 0.0 {
		t.Fatalf("expected value 0.0 at left edge, got %f", s.Value)
	}

	// Release.
	s.HandleInputResult(s.TrackRect().Min.X, 5, false)
	if s.Capturing() {
		t.Fatal("should not be capturing after release")
	}
}

// TestMobileTransportNotBlockedByTouchDeadZone verifies that a mobile touch
// press on the transport area (play button) is NOT blocked by the touch dead
// zone. The dead zone should only apply to the row scroll area.
func TestMobileTransportNotBlockedByTouchDeadZone(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create enough rows to enable scrolling (triggers dead zone on mobile).
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	pb := dv.playBtn()
	if pb == nil || pb.Rect().Empty() {
		t.Skip("play button not visible in mobile layout")
	}

	pr := pb.Rect()
	px, py := pr.Min.X+pr.Dx()/2, pr.Min.Y+pr.Dy()/2

	// Verify the play button is in the transport area (above the row area).
	rowsRect := dv.rowsRect()
	if image.Pt(px, py).In(rowsRect) {
		t.Fatalf("play button (%d,%d) should be above the row area %v", px, py, rowsRect)
	}

	// Track whether play was fired.
	playFired := false
	pb.OnClick = func() { playFired = true }

	// Simulate a real mobile touch press on the play button.
	// Touch override coordinates are at the play button position (outside rows).
	r := SetInputForTest(
		func() (int, int) { return px, py },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(px, py)
	dv.Update()
	r()

	// The tree should have dispatched the press (not blocked by dead zone).
	if dv.tree == nil {
		t.Fatal("tree is nil")
	}
	if !dv.tree.InputHandled() {
		t.Fatal("tree should have dispatched press to transport button (dead zone should not block outside row area)")
	}
	if !playFired {
		t.Fatal("play button should have fired — transport area is outside dead zone")
	}
}

// TestMobileRowTouchStillBlockedByDeadZone verifies that the dead zone still
// blocks row-area touches on mobile when scrolling is possible.
func TestMobileRowTouchStillBlockedByDeadZone(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create enough rows to enable scrolling.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Use row labels (always visible on mobile, unlike kebab buttons).
	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after warm-up")
	}
	labelRect := dv.rowLabels()[0].Rect()
	if labelRect.Empty() {
		t.Skip("row label rect empty")
	}
	lx := labelRect.Min.X + labelRect.Dx()/2
	ly := labelRect.Min.Y + labelRect.Dy()/2

	// Verify the label is in the row area.
	rowsRect := dv.rowsRect()
	if !image.Pt(lx, ly).In(rowsRect) {
		t.Fatalf("row label (%d,%d) should be inside rowsRect %v", lx, ly, rowsRect)
	}

	// Simulate a real mobile touch press on the label.
	r := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(lx, ly)
	dv.Update()
	r()

	// The dead zone should block dispatch — context menu should NOT open.
	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should NOT open during real touch in row area (dead zone should block)")
	}
}

// TestPortalSliderNotBlockedByDeadZone verifies that a volume popup portal
// slider receives touch even when the touch dead zone would otherwise block.
// Portal overlays have z-index >= ZOverlayMin and bypass the dragBlocked guard.
func TestPortalSliderNotBlockedByDeadZone(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create enough rows to enable scrolling.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     0.5,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Open a volume popup portal. This creates an overlay with z >= ZOverlayMin.
	// Use a fallback anchor rect in the row area since mobile may not show
	// row vol sliders directly.
	iconRect := image.Rect(10, dv.rowsRect().Min.Y+10, 50, dv.rowsRect().Min.Y+50)
	if len(dv.rowVolSliders()) > 0 && !dv.rowVolSliders()[0].Rect().Empty() {
		iconRect = dv.rowVolSliders()[0].Rect()
	}
	dv.volPopup.Open(iconRect, dv.Bounds, dv.headerH)
	dv.openVolPopupPortal()

	if !dv.portal().Has("volume-popup") {
		t.Fatal("volume-popup portal should be open")
	}

	// Get the popup slider position. The popup rect is anchored near iconRect.
	popupRect := dv.volPopup.Rect()
	if popupRect.Empty() {
		t.Skip("volume popup rect is empty")
	}
	px, py := popupRect.Min.X+popupRect.Dx()/2, popupRect.Min.Y+popupRect.Dy()/2

	// Simulate a real mobile touch press on the popup slider, with touch
	// coordinates in the row area (activating the dead zone).
	rowsRect := dv.rowsRect()
	// Place touch override in the rows area to trigger dead zone.
	dzY := rowsRect.Min.Y + rowsRect.Dy()/2
	r := SetInputForTest(
		func() (int, int) { return px, py },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(px, dzY)
	dv.Update()
	r()

	// The overlay tree should have dispatched the press to the portal overlay
	// even though the dead zone is active.
	if !dv.overlayTree.InputHandled() {
		t.Fatal("overlay tree should have dispatched press to portal slider (portal bypasses drag block)")
	}
}
