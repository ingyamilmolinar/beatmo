//go:build test

package ui

import (
	"image"
	"testing"
)

func TestMenuScroll_OffsetAndWheel(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	m.Configure(image.Rect(0, 0, 100, 100), 20, 5) // 20 items, 5 visible

	if !m.HasScroll() {
		t.Fatalf("HasScroll()=false; want true (Total=20 Visible=5)")
	}
	if got := m.OffsetPx(); got != 0 {
		t.Fatalf("OffsetPx()=%d; want 0 at top", got)
	}
	// Clicky: one wheel notch moves exactly ONE item regardless of magnitude —
	// a fast trackpad flick / hi-res wheel (|steps|=2) must not fly two rows.
	if !m.HandleWheel(-2) {
		t.Fatalf("HandleWheel returned false; expected a one-item offset change")
	}
	if got := m.OffsetPx(); got != 1*20 {
		t.Fatalf("OffsetPx()=%d; want %d (exactly one item per notch, magnitude ignored)", got, 1*20)
	}
}

// TestMenuScroll_WheelIsClicky pins the menu wheel to the SAME clicky cadence as
// the row rack / control grids: one item per notch (magnitude ignored) plus a
// cooldown lock so a held wheel / fast flick can't fly through the list. The
// cooldown only releases after controlGridScrollCooldownFrames TickStep() calls.
func TestMenuScroll_WheelIsClicky(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	m.Configure(image.Rect(0, 0, 100, 100), 50, 5)

	// First notch: one item, even with a big magnitude.
	if !m.HandleWheel(-3) || m.OffsetPx() != 20 {
		t.Fatalf("first notch: OffsetPx()=%d, want 20 (one item)", m.OffsetPx())
	}
	// Immediately again within the cooldown: no movement.
	if m.HandleWheel(-3) {
		t.Fatalf("second notch within cooldown moved; want locked (OffsetPx=%d)", m.OffsetPx())
	}
	if m.OffsetPx() != 20 {
		t.Fatalf("OffsetPx()=%d after locked notch; want still 20", m.OffsetPx())
	}
	// Advance the cooldown clock, then it releases exactly one more item.
	for i := 0; i < controlGridScrollCooldownFrames; i++ {
		m.TickStep()
	}
	if !m.HandleWheel(-3) || m.OffsetPx() != 40 {
		t.Fatalf("after cooldown: OffsetPx()=%d, want 40 (one more item)", m.OffsetPx())
	}
	// Zero-magnitude notch is a no-op.
	for i := 0; i < controlGridScrollCooldownFrames; i++ {
		m.TickStep()
	}
	if m.HandleWheel(0) {
		t.Fatalf("HandleWheel(0) moved; want no-op")
	}
}

func TestMenuScroll_NoScrollWhenFits(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	m.Configure(image.Rect(0, 0, 100, 100), 3, 5) // fewer items than visible
	if m.HasScroll() {
		t.Fatalf("HasScroll()=true; want false when content fits")
	}
	if got := m.OffsetPx(); got != 0 {
		t.Fatalf("OffsetPx()=%d; want 0 when no scroll", got)
	}
	if m.HandleWheel(-2) {
		t.Fatalf("HandleWheel returned true when content fits; expected no-op")
	}
}

func TestMenuScroll_HandleScrollbarDrag(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	view := image.Rect(0, 0, 100, 100)
	m.Configure(view, 20, 5)

	thumb := m.ScrollBehavior().ThumbRect()
	relayouts := 0
	// Press inside the thumb starts the drag (returns true, consumed).
	if !m.HandleScrollbarDrag(image.Pt(thumb.Min.X+1, thumb.Min.Y+1), true, func() { relayouts++ }) {
		t.Fatalf("press in thumb not consumed by HandleScrollbarDrag")
	}
	// Drag to the bottom moves the offset and fires relayout.
	m.HandleScrollbarDrag(image.Pt(thumb.Min.X+1, view.Max.Y-1), true, func() { relayouts++ })
	if m.OffsetPx() == 0 {
		t.Fatalf("OffsetPx still 0 after dragging thumb to bottom")
	}
	if relayouts == 0 {
		t.Fatalf("relayout never called during scrollbar drag")
	}
	// Release ends the drag.
	m.HandleScrollbarDrag(image.Pt(thumb.Min.X+1, view.Max.Y-1), false, nil)
	if m.ScrollBehavior().Dragging() {
		t.Fatalf("still dragging after release")
	}
	// A nil relayout must be safe (no panic) on a fresh press.
	m.HandleScrollbarDrag(image.Pt(thumb.Min.X+1, thumb.Min.Y+1), true, nil)
}

// withSmallScreenReturn sets the mobile/desktop screen class for the test and
// returns a restore func. Thin wrapper so MenuScroll tests can toggle class in
// BOTH directions; it mirrors the forceSmallScreenForTest + UpdateProfile
// mechanism used by withSmallScreen, but without the "must start false"
// assertion so it can be called for either class.
func withSmallScreenReturn(t *testing.T, small bool) func() {
	t.Helper()
	prev := forceSmallScreenForTest
	forceSmallScreenForTest = small
	UpdateProfile()
	return func() {
		forceSmallScreenForTest = prev
		UpdateProfile()
	}
}

func TestMenuScroll_HandleInput_DesktopThumbDrag(t *testing.T) {
	restore := withSmallScreenReturn(t, false) // desktop
	defer restore()

	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	view := image.Rect(0, 0, 100, 100)
	m.Configure(view, 20, 5)

	thumb := m.ScrollBehavior().ThumbRect()
	relayouts := 0
	// Press inside the thumb starts a drag.
	if !m.HandleInput(MenuScrollInput{
		Pt: image.Pt(thumb.Min.X+1, thumb.Min.Y+1), Pressed: true,
		PopupRect: view, ItemView: view,
		Relayout: func() { relayouts++ },
	}) {
		t.Fatalf("press in thumb not handled")
	}
	// Drag down moves the offset.
	m.HandleInput(MenuScrollInput{
		Pt: image.Pt(thumb.Min.X+1, view.Max.Y-1), Pressed: true,
		PopupRect: view, ItemView: view,
		Relayout: func() { relayouts++ },
	})
	if m.OffsetPx() == 0 {
		t.Fatalf("OffsetPx still 0 after dragging thumb to bottom")
	}
	if relayouts == 0 {
		t.Fatalf("Relayout never called during drag")
	}
}

func TestMenuScroll_HandleInput_DesktopButtonHit(t *testing.T) {
	restore := withSmallScreenReturn(t, false) // desktop
	defer restore()

	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	view := image.Rect(0, 0, 100, 200)
	m.Configure(view, 3, 5) // no scroll

	hit := false
	handled := m.HandleInput(MenuScrollInput{
		Pt: image.Pt(10, 10), Pressed: true,
		PopupRect: view, ItemView: view,
		DesktopHit: func(pt image.Point, pressed bool) bool { hit = true; return true },
	})
	if !handled || !hit {
		t.Fatalf("DesktopHit not invoked (handled=%v hit=%v)", handled, hit)
	}
}
