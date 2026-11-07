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
	// Magnitude respected: |steps|=2 moves two rows in one event.
	if !m.HandleWheel(-2) {
		t.Fatalf("HandleWheel returned false; expected a two-item offset change")
	}
	if got := m.OffsetPx(); got != 2*20 {
		t.Fatalf("OffsetPx()=%d; want %d (magnitude respected: two items)", got, 2*20)
	}
}

// No per-notch cooldown, so consecutive notches scroll on consecutive frames.
func TestMenuScroll_WheelIsSmooth(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	m.Configure(image.Rect(0, 0, 100, 100), 50, 5)

	// Magnitude respected: a three-step notch moves three items at once.
	if !m.HandleWheel(-3) || m.OffsetPx() != 3*20 {
		t.Fatalf("first notch: OffsetPx()=%d, want 60 (three items, magnitude respected)", m.OffsetPx())
	}
	// No cooldown lock: an immediately-following notch moves again, no TickStep.
	if !m.HandleWheel(-2) || m.OffsetPx() != 5*20 {
		t.Fatalf("second notch (no cooldown): OffsetPx()=%d, want 100", m.OffsetPx())
	}
	// Zero-magnitude notch is a no-op.
	if m.HandleWheel(0) {
		t.Fatalf("HandleWheel(0) moved; want no-op")
	}
}

// A no-op at rest; after a fling it carries the list on, rebuilding on each moved frame. A nil rebuild is safe.
func TestMenuScroll_TickDrivesMomentumAndRebuilds(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	m.Configure(image.Rect(0, 0, 100, 100), 200, 5)
	sb := m.ScrollBehavior()

	rebuilds := 0
	if m.Tick(func() { rebuilds++ }) || rebuilds != 0 {
		t.Fatalf("Tick at rest moved/rebuilt (rebuilds=%d)", rebuilds)
	}

	// Fast upward fling → content scrolls down with inertia after release.
	sb.HandleTouchBegin(50, 90)
	sb.HandleTouchMove(50, 60) // commit vertical direction past the dead zone
	sb.HandleTouchMove(50, 20) // build velocity
	firstAtRelease := sb.VS.First
	sb.HandleTouchEnd()

	moved := false
	for i := 0; i < 600 && sb.HasMomentum(); i++ {
		if m.Tick(func() { rebuilds++ }) {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("Tick never advanced the offset during the fling")
	}
	if rebuilds == 0 {
		t.Fatalf("Tick never invoked rebuild while momentum moved the offset")
	}
	if sb.VS.First <= firstAtRelease {
		t.Fatalf("momentum did not carry past release (First=%d release=%d)", sb.VS.First, firstAtRelease)
	}
	m.Tick(nil) // nil rebuild must not panic
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

// Either direction, unlike withSmallScreen, which must start from false.
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
