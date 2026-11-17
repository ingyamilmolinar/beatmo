//go:build test

package ui

import (
	"image"
	"testing"
)

// TestInputCaptureConsumesPress pins the core contract: any press
// inside the capture rect returns InputConsumed so the tree's
// dispatcher stops at this hit area instead of falling through to a
// lower-z sibling.
func TestInputCaptureConsumesPress(t *testing.T) {
	h := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "test")
	if h.Handler == nil {
		t.Fatalf("NewInputCaptureHitArea returned area with nil handler")
	}
	if got := h.Handler.OnPress(10, 10); got != InputConsumed {
		t.Errorf("OnPress() = %v, want InputConsumed", got)
	}
}

// TestInputCaptureIgnoresWheel pins the wheel-bubble contract: scroll
// passes through the catch-all so a zone that wraps a scrollable
// region (rack, future Synth scroller) keeps working. A catch-all
// that swallowed wheels would break touchpad scrolling over the
// audio panel.
func TestInputCaptureIgnoresWheel(t *testing.T) {
	h := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "test")
	if got := h.Handler.OnWheel(10, 10, 3); got != InputIgnored {
		t.Errorf("OnWheel() = %v, want InputIgnored", got)
	}
}

// TestInputCaptureDragAndReleaseAreNoops: a "consumed" press never
// reaches the captured-handler path in the tree, so OnDrag/OnRelease
// are never called for this handler in production. Pinning here
// catches anyone who refactors the handler into a stateful drag
// adapter without updating the docstring.
func TestInputCaptureDragAndReleaseAreNoops(t *testing.T) {
	h := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "test")
	// Should not panic, should not mutate any global state.
	h.Handler.OnDrag(10, 10)
	h.Handler.OnRelease(10, 10)
}

// TestInputCaptureSharedHandler — the helper reuses a single stateless
// handler so rebuildHitAreas hot loops don't allocate. Two areas
// created back-to-back must share the same Handler interface pointer
// value (==), proving the singleton.
func TestInputCaptureSharedHandler(t *testing.T) {
	a := NewInputCaptureHitArea(image.Rect(0, 0, 100, 50), 130, "a")
	b := NewInputCaptureHitArea(image.Rect(0, 0, 200, 60), 140, "b")
	if a.Handler != b.Handler {
		t.Errorf("expected shared handler singleton, got distinct values: %p vs %p", a.Handler, b.Handler)
	}
}

// TestInputCaptureCarriesRectAndZ — sanity check: the factory wires
// the rect / z / tag through verbatim. Catches a future refactor that
// accidentally mutates them.
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
