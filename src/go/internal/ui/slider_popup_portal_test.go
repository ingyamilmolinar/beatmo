//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// helper: create an open SliderPopup and its hit handler, with suppressClicksUntilRelease cleared.
func openSliderPopupWithHandler(t *testing.T, val *float64) (*SliderPopup, *sliderPopupHitHandler) {
	t.Helper()
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-portal",
		ZIndex:   100,
		GetValue: func() float64 { return *val },
		SetValue: func(v float64) { *val = v },
	})
	anchor := image.Rect(100, 300, 130, 330)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)
	// Open() sets suppressClicksUntilRelease; clear it so HandleInput works in tests.
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })
	return sp, &sliderPopupHitHandler{popup: sp}
}

func TestSliderPopupHitHandler_OnPressCaptures(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	r := sp.Rect()
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	result := handler.OnPress(cx, cy)
	if result != InputCaptured {
		t.Fatalf("OnPress inside popup returned %v, want InputCaptured", result)
	}
	if !sp.IsDragging() {
		t.Fatal("popup should be dragging after OnPress inside")
	}
}

func TestSliderPopupHitHandler_OnPressOutsideIgnored(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	// Press far outside the popup rect.
	result := handler.OnPress(0, 0)
	if result != InputIgnored {
		t.Fatalf("OnPress outside popup returned %v, want InputIgnored", result)
	}
	if sp.IsDragging() {
		t.Fatal("popup should not be dragging after OnPress outside")
	}
}

func TestSliderPopupHitHandler_OnDragUpdatesValue(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	r := sp.Rect()
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	// Start drag inside.
	result := handler.OnPress(cx, cy)
	if result != InputCaptured {
		t.Fatalf("OnPress returned %v, want InputCaptured", result)
	}
	valAfterPress := val

	// Drag to a different Y (near top of popup should increase value toward 1.0).
	handler.OnDrag(cx, r.Min.Y)
	if val == valAfterPress {
		t.Fatal("value should have changed after OnDrag to different Y")
	}
}

func TestSliderPopupHitHandler_OnReleaseEndsDrag(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	r := sp.Rect()
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	// Start drag.
	handler.OnPress(cx, cy)
	if !sp.IsDragging() {
		t.Fatal("should be dragging after OnPress")
	}

	// Release.
	handler.OnRelease(cx, cy)
	if sp.IsDragging() {
		t.Fatal("should not be dragging after OnRelease")
	}
}

func TestSliderPopupHitHandler_OnWheelAlwaysConsumed(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	_, handler := openSliderPopupWithHandler(t, &val)

	// Wheel at various positions/steps should always return InputConsumed.
	for _, tc := range []struct{ x, y, steps int }{
		{0, 0, 1},
		{100, 200, -3},
		{999, 999, 10},
	} {
		result := handler.OnWheel(tc.x, tc.y, tc.steps)
		if result != InputConsumed {
			t.Fatalf("OnWheel(%d,%d,%d) returned %v, want InputConsumed", tc.x, tc.y, tc.steps, result)
		}
	}
}

func TestSliderPopupPortalOverlay_LayoutIsNoOp(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-layout-noop",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	anchor := image.Rect(100, 300, 130, 330)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "layout-test"}

	rectBefore := sp.Rect()
	openBefore := sp.IsOpen()

	// Layout should be a no-op regardless of arguments.
	overlay.Layout(image.Rect(0, 0, 50, 50), image.Rect(0, 0, 1920, 1080))

	if sp.Rect() != rectBefore {
		t.Fatalf("Layout changed popup rect from %v to %v", rectBefore, sp.Rect())
	}
	if sp.IsOpen() != openBefore {
		t.Fatalf("Layout changed open state from %v to %v", openBefore, sp.IsOpen())
	}
}

func TestSliderPopupPortalOverlay_DrawDelegates(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-draw",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	anchor := image.Rect(100, 300, 130, 330)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "draw-test"}

	// Draw to a real image — should not panic.
	screen := ebiten.NewImage(800, 600)
	overlay.Draw(screen)

	// Also draw when closed — should not panic.
	sp.Close()
	overlay.Draw(screen)
}

func TestSliderPopupHitHandler_FullDragCycle(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	r := sp.Rect()
	cx := r.Min.X + r.Dx()/2
	trackTop := sp.trackTop()
	trackBot := sp.trackBot()

	// Start drag at center of popup.
	cy := r.Min.Y + r.Dy()/2
	result := handler.OnPress(cx, cy)
	if result != InputCaptured {
		t.Fatalf("OnPress returned %v, want InputCaptured", result)
	}

	// Drag to top of track → value should approach 1.0.
	handler.OnDrag(cx, trackTop)
	if val != 1.0 {
		t.Fatalf("after drag to trackTop, val = %f, want 1.0", val)
	}

	// Drag to bottom of track → value should approach 0.0.
	handler.OnDrag(cx, trackBot)
	if val != 0.0 {
		t.Fatalf("after drag to trackBot, val = %f, want 0.0", val)
	}

	// Drag to middle → value should be ~0.5.
	mid := (trackTop + trackBot) / 2
	handler.OnDrag(cx, mid)
	if math.Abs(val-0.5) > 0.02 {
		t.Fatalf("after drag to midpoint, val = %f, want ~0.5", val)
	}

	// Release — drag should end.
	handler.OnRelease(cx, mid)
	if sp.IsDragging() {
		t.Fatal("should not be dragging after OnRelease")
	}
}

func TestSliderPopupHitHandler_OnPressContinuesDrag(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	r := sp.Rect()
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	// Initial press starts drag.
	handler.OnPress(cx, cy)
	if !sp.IsDragging() {
		t.Fatal("should be dragging after first OnPress")
	}

	// Second press outside popup rect but while dragging → still captured
	// because HandleInput checks sp.dragging first.
	result := handler.OnPress(0, 0)
	if result != InputCaptured {
		t.Fatalf("OnPress while dragging (even outside rect) returned %v, want InputCaptured", result)
	}
}

func TestSliderPopupHitHandler_OnPressConsumedNotCaptured(t *testing.T) {
	// Test the InputConsumed branch: HandleInput returns true but IsDragging is false.
	// This can't happen in normal flow because HandleInput with pressed=true + inside rect
	// always sets dragging=true. So we test the release path where HandleInput returns false.
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	// When the popup is open but press is outside, HandleInput returns false → InputIgnored.
	result := handler.OnPress(0, 0)
	if result != InputIgnored {
		t.Fatalf("OnPress outside returned %v, want InputIgnored", result)
	}
	if sp.IsDragging() {
		t.Fatal("should not be dragging")
	}
}

func TestSliderPopupPortalOverlay_HitAreasClosedEmpty(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-closed",
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "closed-tag"}

	// Popup not opened → rect is empty → no hit areas.
	areas := overlay.HitAreas()
	if len(areas) != 0 {
		t.Fatalf("expected 0 hit areas when popup not opened, got %d", len(areas))
	}
}

func TestSliderPopupPortalOverlay_HitAreasOpenHasTag(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-tag",
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	sp.Open(image.Rect(100, 100, 120, 120), image.Rect(0, 0, 800, 600), 40)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "my-tag"}
	areas := overlay.HitAreas()
	if len(areas) != 1 {
		t.Fatalf("expected 1 hit area, got %d", len(areas))
	}
	if areas[0].Tag != "my-tag" {
		t.Errorf("expected tag 'my-tag', got %q", areas[0].Tag)
	}
	if areas[0].Rect != sp.Rect() {
		t.Errorf("hit area rect %v != popup rect %v", areas[0].Rect, sp.Rect())
	}
	// Verify the handler is a *sliderPopupHitHandler.
	if _, ok := areas[0].Handler.(*sliderPopupHitHandler); !ok {
		t.Fatalf("expected handler to be *sliderPopupHitHandler, got %T", areas[0].Handler)
	}
}

func TestSliderPopupPortalOverlay_ShouldCloseReflectsPopupState(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-shouldclose",
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "sc"}

	// Not opened → should close.
	if !overlay.ShouldClose() {
		t.Fatal("ShouldClose should return true when popup is not open")
	}

	sp.Open(image.Rect(100, 100, 120, 120), image.Rect(0, 0, 800, 600), 40)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	// Opened → should not close.
	if overlay.ShouldClose() {
		t.Fatal("ShouldClose should return false when popup is open")
	}

	sp.Close()

	// Closed → should close.
	if !overlay.ShouldClose() {
		t.Fatal("ShouldClose should return true after popup is closed")
	}
}

func TestSliderPopupHitHandler_DragBeyondTrackClampsValue(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	r := sp.Rect()
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	// Start drag.
	handler.OnPress(cx, cy)

	// Drag far above the popup → value should clamp to 1.0.
	handler.OnDrag(cx, r.Min.Y-500)
	if val != 1.0 {
		t.Fatalf("drag far above: val = %f, want 1.0", val)
	}

	// Drag far below the popup → value should clamp to 0.0.
	handler.OnDrag(cx, r.Max.Y+500)
	if val != 0.0 {
		t.Fatalf("drag far below: val = %f, want 0.0", val)
	}

	handler.OnRelease(cx, r.Max.Y+500)
}

func TestSliderPopupHitHandler_OnReleaseWhenNotDragging(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp, handler := openSliderPopupWithHandler(t, &val)

	// OnRelease without a prior press should not panic or change state.
	handler.OnRelease(0, 0)
	if sp.IsDragging() {
		t.Fatal("should not be dragging after OnRelease without prior press")
	}
	if val != 0.5 {
		t.Fatalf("value should remain 0.5, got %f", val)
	}
}

func TestSliderPopupPortalOverlay_HitAreasTouchFlag(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-touch-flag",
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	sp.Open(image.Rect(100, 300, 130, 330), image.Rect(0, 0, 800, 600), 30)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "touch-flag-test"}
	areas := overlay.HitAreas()
	if len(areas) != 1 {
		t.Fatalf("expected 1 hit area, got %d", len(areas))
	}
	if !areas[0].Touch {
		t.Fatal("slider popup portal overlay HitArea must have Touch == true for mobile touch expansion")
	}
}

func TestSliderPopupPortalOverlay_TouchExpandedHit(t *testing.T) {
	assertDefaultParityState(t)

	forceSmallScreenForTest = true
	UpdateProfile()
	defer func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	}()

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-touch-expand",
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	sp.Open(image.Rect(100, 300, 130, 330), image.Rect(0, 0, 800, 600), 30)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "touch-expand-test"}
	areas := overlay.HitAreas()
	if len(areas) != 1 {
		t.Fatalf("expected 1 hit area, got %d", len(areas))
	}

	expand := TouchMinTarget()
	if expand <= 0 {
		t.Fatal("expected positive TouchMinTarget on small screen")
	}

	idx := &HitIndex{}
	idx.UpdatePortal("slider-overlay", areas)

	r := sp.Rect()
	// Query just outside the exact rect but within touch expansion.
	hits := idx.At(r.Min.X-expand+1, r.Min.Y+r.Dy()/2)
	if len(hits) == 0 {
		t.Fatal("expected touch-expanded hit just outside popup rect, got 0 hits")
	}
	if hits[0].Tag != "touch-expand-test" {
		t.Errorf("expected tag 'touch-expand-test', got %q", hits[0].Tag)
	}
}

func TestSliderPopupPortal_TouchDragIntegration(t *testing.T) {
	assertDefaultParityState(t)

	forceSmallScreenForTest = true
	UpdateProfile()
	defer func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	}()

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-touch-drag",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	sp.Open(image.Rect(100, 300, 130, 330), image.Rect(0, 0, 800, 600), 30)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	overlay := &sliderPopupPortalOverlay{popup: sp, tag: "touch-drag-test"}
	areas := overlay.HitAreas()

	idx := &HitIndex{}
	idx.UpdatePortal("slider-overlay", areas)
	idx.SetModal("slider-overlay")

	r := sp.Rect()
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	// Press inside exact rect → handler should capture.
	hits := idx.At(cx, cy)
	if len(hits) == 0 {
		t.Fatal("expected hit inside popup rect")
	}
	result := hits[0].Handler.OnPress(cx, cy)
	if result != InputCaptured {
		t.Fatalf("OnPress returned %v, want InputCaptured", result)
	}
	if !sp.IsDragging() {
		t.Fatal("popup should be dragging after OnPress")
	}

	// Drag to top of track → value should approach 1.0.
	trackTop := sp.trackTop()
	hits[0].Handler.OnDrag(cx, trackTop)
	if val != 1.0 {
		t.Fatalf("after drag to trackTop, val = %f, want 1.0", val)
	}

	// Release.
	hits[0].Handler.OnRelease(cx, trackTop)
	if sp.IsDragging() {
		t.Fatal("should not be dragging after OnRelease")
	}
}
