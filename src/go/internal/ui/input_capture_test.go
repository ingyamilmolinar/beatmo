//go:build test

package ui

import (
	"image"
	"testing"
)

// Consumed, so dispatch stops here instead of falling through to a lower-z sibling.
func TestInputCaptureConsumesPress(t *testing.T) {
	h := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "test")
	if h.Handler == nil {
		t.Fatalf("NewInputCaptureHitArea returned area with nil handler")
	}
	if got := h.Handler.OnPress(10, 10); got != InputConsumed {
		t.Errorf("OnPress() = %v, want InputConsumed", got)
	}
}

// Wheels pass through, or touchpad scrolling over a scrollable zone would break.
func TestInputCaptureIgnoresWheel(t *testing.T) {
	h := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "test")
	if got := h.Handler.OnWheel(10, 10, 3); got != InputIgnored {
		t.Errorf("OnWheel() = %v, want InputIgnored", got)
	}
}

// A consumed press never reaches the captured-handler path, so these are never called in production.
func TestInputCaptureDragAndReleaseAreNoops(t *testing.T) {
	h := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "test")
	// Should not panic, should not mutate any global state.
	h.Handler.OnDrag(10, 10)
	h.Handler.OnRelease(10, 10)
}

// One shared stateless handler, so rebuildHitAreas hot loops do not allocate.
func TestInputCaptureSharedHandler(t *testing.T) {
	a := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "a")
	b := NewInputCaptureHitArea(image.Rect(0, 0, 200, 60), 140, "b")
	if a.Handler != b.Handler {
		t.Errorf("expected shared handler singleton, got distinct values: %p vs %p", a.Handler, b.Handler)
	}
}

func TestInputCaptureCarriesRectAndZ(t *testing.T) {
	r := image.Rect(10, 20, 200, 100)
	h := NewInputCaptureHitArea(r, 130, "synth-capture")
	if h.Rect != r {
		t.Errorf("Rect=%v, want %v", h.Rect, r)
	}
	if h.ZIndex != 130 {
		t.Errorf("ZIndex=%d, want 130", h.ZIndex)
	}
	if h.Tag != "synth-capture" {
		t.Errorf("Tag=%q, want %q", h.Tag, "synth-capture")
	}
}
