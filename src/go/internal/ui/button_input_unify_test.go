//go:build test

package ui

import (
	"image"
	"testing"
)

// TestButtonHitAdapterDrivesPressLifecycle pins the unified contract: the shared
// buttonHitAdapter must drive the Button's full press lifecycle, not just fire
// OnClick blind. Press sets the visual pressed state and fires once; release
// clears it; a second press fires again (no stale-held latch). Before the
// unification the adapter fired OnClick directly and never touched the Button's
// pressed state, so buttons gave no press feedback and bypassed the release path.
func TestButtonHitAdapterDrivesPressLifecycle(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	fires := 0
	b := NewButton("ok", ButtonStyle{}, func() { fires++ })
	b.SetRect(image.Rect(0, 0, 50, 20))
	h := &buttonHitAdapter{btn: b}

	// Press: fires once and the button reads as pressed (visual feedback).
	if r := h.OnPress(25, 10); r != InputCaptured {
		t.Fatalf("OnPress returned %d, want InputCaptured", r)
	}
	if fires != 1 {
		t.Fatalf("OnPress fired OnClick %d times, want 1", fires)
	}
	if !b.Pressed() {
		t.Fatal("after OnPress the button must read as pressed (no visual feedback otherwise)")
	}

	// Release: clears the pressed state.
	h.OnRelease(25, 10)
	if b.Pressed() {
		t.Fatal("after OnRelease the button must no longer read as pressed")
	}

	// Second press fires again — the press edge is never swallowed by a stale
	// held counter even though the same *Button is reused.
	h.OnPress(25, 10)
	if fires != 2 {
		t.Fatalf("second OnPress fired total=%d, want 2 (stale-held latch swallowed it)", fires)
	}
	h.OnRelease(25, 10)
}

// TestButtonHitAdapterDisabledSwallows verifies a disabled button consumes the
// press (no fall-through to a lower-z sibling) but never fires.
func TestButtonHitAdapterDisabledSwallows(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("x", ButtonStyle{}, func() { t.Fatal("disabled button must not fire") })
	b.SetRect(image.Rect(0, 0, 50, 20))
	b.Disabled = true
	h := &buttonHitAdapter{btn: b}

	if r := h.OnPress(25, 10); r != InputConsumed {
		t.Fatalf("disabled OnPress returned %d, want InputConsumed", r)
	}
}
