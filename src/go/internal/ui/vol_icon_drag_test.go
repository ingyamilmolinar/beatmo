//go:build test

package ui

import (
	"image"
	"testing"
)

// TestRowVolIconOnPressReturnsInputCaptured verifies that OnPress returns
// InputCaptured (not InputConsumed) so the hit tree routes drag/release events.
func TestRowVolIconOnPressReturnsInputCaptured(t *testing.T) {
	assertDefaultParityState(t)

	h := &rowVolIconHitAdapter{onClick: func() {}}
	r := h.OnPress(5, 5)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
}

// TestRowVolIconDragForwardsToPopup verifies that dragging after press
// forwards to the SliderPopup, enabling tap-and-slide in one gesture.
func TestRowVolIconDragForwardsToPopup(t *testing.T) {
	assertDefaultParityState(t)

	var setVal float64
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-vol",
		GetValue: func() float64 { return 0.5 },
		SetValue: func(v float64) { setVal = v },
	})
	// Open the popup so HandleInput accepts input.
	sp.Open(image.Rect(100, 200, 150, 220), image.Rect(0, 0, 400, 600), 0)

	h := &rowVolIconHitAdapter{
		onClick: func() {},
		popup:   sp,
	}

	// Press on icon.
	h.OnPress(120, 210)

	// Drag into popup rect — should set value and start dragging.
	r := sp.Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	h.OnDrag(mx, my)

	if !sp.IsDragging() {
		t.Fatal("expected popup to be dragging after OnDrag inside popup rect")
	}
	if setVal == 0.5 {
		t.Fatal("expected SetValue to be called with a different value")
	}
}

// TestRowVolIconDragOutsidePopupNoValueJump verifies that dragging while
// outside the popup rect does not change the value (no jump on open).
func TestRowVolIconDragOutsidePopupNoValueJump(t *testing.T) {
	assertDefaultParityState(t)

	var setVal float64
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-vol",
		GetValue: func() float64 { return 0.5 },
		SetValue: func(v float64) { setVal = v },
	})
	sp.Open(image.Rect(100, 50, 150, 70), image.Rect(0, 0, 400, 600), 0)

	h := &rowVolIconHitAdapter{
		onClick: func() {},
		popup:   sp,
	}

	h.OnPress(120, 60)

	// Drag far outside popup rect.
	h.OnDrag(0, 0)

	if sp.IsDragging() {
		t.Fatal("dragging should not start when point is outside popup rect")
	}
	if setVal != 0 {
		t.Fatal("SetValue should not have been called for drag outside popup")
	}
}

// TestRowVolIconReleaseForwardsToPopup verifies that releasing after a drag
// stops the popup's drag state.
func TestRowVolIconReleaseForwardsToPopup(t *testing.T) {
	assertDefaultParityState(t)

	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-vol",
		GetValue: func() float64 { return 0.5 },
		SetValue: func(v float64) {},
	})
	sp.Open(image.Rect(100, 200, 150, 220), image.Rect(0, 0, 400, 600), 0)

	h := &rowVolIconHitAdapter{
		onClick: func() {},
		popup:   sp,
	}

	h.OnPress(120, 210)

	// Drag into popup to start dragging.
	r := sp.Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	h.OnDrag(mx, my)
	if !sp.IsDragging() {
		t.Fatal("expected dragging to be true after OnDrag")
	}

	// Release — dragging should stop.
	h.OnRelease(mx, my)
	if sp.IsDragging() {
		t.Fatal("expected dragging to be false after OnRelease")
	}
}

// TestTransportVolIconDragForwardsToPopup verifies the master volume icon
// adapter forwards drag events to its popup.
func TestTransportVolIconDragForwardsToPopup(t *testing.T) {
	assertDefaultParityState(t)

	var setVal float64
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-master-vol",
		GetValue: func() float64 { return 0.5 },
		SetValue: func(v float64) { setVal = v },
	})
	sp.Open(image.Rect(100, 200, 150, 220), image.Rect(0, 0, 400, 600), 0)

	h := &transportVolIconHitAdapter{
		onClick: func() {},
		popup:   sp,
	}

	h.OnPress(120, 210)

	r := sp.Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	h.OnDrag(mx, my)

	if !sp.IsDragging() {
		t.Fatal("expected popup to be dragging after OnDrag inside popup rect")
	}
	if setVal == 0.5 {
		t.Fatal("expected SetValue to be called with a different value")
	}
}

// TestTransportVolIconReleaseStopsDrag verifies that releasing after a drag
// stops the master volume popup's drag state.
func TestTransportVolIconReleaseStopsDrag(t *testing.T) {
	assertDefaultParityState(t)

	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-master-vol",
		GetValue: func() float64 { return 0.5 },
		SetValue: func(v float64) {},
	})
	sp.Open(image.Rect(100, 200, 150, 220), image.Rect(0, 0, 400, 600), 0)

	h := &transportVolIconHitAdapter{
		onClick: func() {},
		popup:   sp,
	}

	h.OnPress(120, 210)

	r := sp.Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	h.OnDrag(mx, my)
	if !sp.IsDragging() {
		t.Fatal("expected dragging to be true after OnDrag")
	}

	h.OnRelease(mx, my)
	if sp.IsDragging() {
		t.Fatal("expected dragging to be false after OnRelease")
	}
}

// TestVolIconDragNilPopupNoPanic verifies that both adapters with nil popup
// do not panic on OnDrag/OnRelease.
func TestVolIconDragNilPopupNoPanic(t *testing.T) {
	assertDefaultParityState(t)

	// Row adapter with nil popup.
	rh := &rowVolIconHitAdapter{onClick: func() {}, popup: nil}
	rh.OnDrag(10, 10)
	rh.OnRelease(10, 10)

	// Transport adapter with nil popup.
	th := &transportVolIconHitAdapter{onClick: func() {}, popup: nil}
	th.OnDrag(10, 10)
	th.OnRelease(10, 10)
}
