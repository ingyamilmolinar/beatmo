//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newSmallDrumView creates a DrumView with a single row and a small screen
// (callers pass small heights, e.g. 180px) to force context menu overflow.
// Returns the DrumView after a warm-up frame.
func newSmallDrumView(t *testing.T, w, h int) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	bounds := image.Rect(0, 0, w, h)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame.
	resetWarm := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	resetWarm()
	return dv
}

// TestContextMenuScrollStateOnOverflow verifies that opening the context menu
// on a small screen results in a scrollable state. Height is sized to force
// at least one hidden item with the current 3-item mobile menu
// (Rename / Origin / Delete + header).
func TestContextMenuScrollStateOnOverflow(t *testing.T) {
	dv := newSmallDrumView(t, 390, 120)
	dv.OpenContextMenu(0)

	scroll := dv.ContextMenuScrollForTest()
	if scroll == nil {
		t.Fatal("contextMenuScroll is nil after opening context menu")
	}
	if !scroll.HasScroll() {
		t.Errorf("expected HasScroll()=true on small screen; Total=%d Visible=%d",
			scroll.VS.Total, scroll.VS.Visible)
	}
	if scroll.VS.Total <= scroll.VS.Visible {
		t.Errorf("expected Total > Visible; Total=%d Visible=%d",
			scroll.VS.Total, scroll.VS.Visible)
	}
}

// TestContextMenuScrollViewExcludesHeader verifies that the scroll viewport
// starts below the header on mobile, not at the menu top.
func TestContextMenuScrollViewExcludesHeader(t *testing.T) {
	dv := newSmallDrumView(t, 390, 180)
	dv.OpenContextMenu(0)

	scroll := dv.ContextMenuScrollForTest()
	if scroll == nil {
		t.Fatal("contextMenuScroll is nil")
	}
	menuRect := dv.ContextMenuRectVal()
	viewRect := scroll.VS.View

	if viewRect.Min.Y <= menuRect.Min.Y {
		t.Errorf("scroll viewport should start below header; view.Min.Y=%d menu.Min.Y=%d",
			viewRect.Min.Y, menuRect.Min.Y)
	}
}

// TestContextMenuWheelScroll verifies that wheel events change the scroll offset.
func TestContextMenuWheelScroll(t *testing.T) {
	dv := newSmallDrumView(t, 390, 180)
	dv.OpenContextMenu(0)

	scroll := dv.ContextMenuScrollForTest()
	if scroll == nil || !scroll.HasScroll() {
		t.Skip("no scroll needed on this screen size")
	}

	before := scroll.VS.First
	// Wheel down (negative steps in HandleWheel scrolls toward higher indices).
	scroll.HandleWheel(-1)
	dv.rebuildContextMenuButtons()

	if scroll.VS.First == before {
		t.Errorf("expected VS.First to change after wheel event; still %d", before)
	}
}

// TestContextMenuTouchScroll verifies that touch drag scrolling works and
// repositions buttons via rebuildContextMenuButtons.
func TestContextMenuTouchScroll(t *testing.T) {
	dv := newSmallDrumView(t, 390, 180)
	dv.OpenContextMenu(0)

	scroll := dv.ContextMenuScrollForTest()
	if scroll == nil || !scroll.HasScroll() {
		t.Skip("no scroll needed on this screen size")
	}

	// Simulate touch inside item area (below header).
	itemView := scroll.VS.View
	sx := itemView.Min.X + itemView.Dx()/2
	sy := itemView.Min.Y + 20

	// Touch begin.
	scroll.HandleTouchBegin(sx, sy)

	// Drag vertically past dead zone (tapMaxMovePx is ~8px).
	dragY := sy - 60
	scroll.HandleTouchMove(sx, dragY)

	if scroll.VS.First <= 0 {
		// Force at least one more big drag.
		scroll.HandleTouchMove(sx, dragY-60)
	}

	scroll.HandleTouchEnd()
	dv.rebuildContextMenuButtons()

	if scroll.VS.First <= 0 {
		t.Errorf("expected VS.First > 0 after touch scroll; got %d", scroll.VS.First)
	}
}

// TestContextMenuTouchScrollCancelsDeferredTap verifies that dragging past the
// dead zone cancels the deferred tap, so releasing doesn't fire a button.
func TestContextMenuTouchScrollCancelsDeferredTap(t *testing.T) {
	dv := newSmallDrumView(t, 390, 180)
	dv.OpenContextMenu(0)

	scroll := dv.ContextMenuScrollForTest()
	if scroll == nil || !scroll.HasScroll() {
		t.Skip("no scroll needed on this screen size")
	}

	// Simulate press inside item area.
	itemView := scroll.VS.View
	sx := itemView.Min.X + itemView.Dx()/2
	sy := itemView.Min.Y + 10

	// Press (starts deferred tap + touch scroll).
	dv.handleContextMenuInput(sx, sy, true)

	// Drag past dead zone.
	dragY := sy - 60
	dv.handleContextMenuInput(sx, dragY, true)

	// Release — should NOT fire the button tap.
	dv.handleContextMenuInput(sx, dragY, false)

	// Menu should still be open (no button was fired to close it).
	if !dv.ContextMenuOpen() {
		t.Error("context menu should still be open after scroll-then-release (no button fired)")
	}
	// Deferred tap should not be active.
	if dv.contextMenuDeferredTap.Active() {
		t.Error("deferred tap should not be active after scroll commit + release")
	}
}

// TestContextMenuTapStillWorks verifies that a simple tap (press+release at
// same position) on a visible button fires its action.
func TestContextMenuTapStillWorks(t *testing.T) {
	dv := newSmallDrumView(t, 390, 180)
	dv.OpenContextMenu(0)

	// openContextMenu sets suppressClicksUntilRelease. Simulate a release
	// to clear the suppression, as would happen in a real touch flow.
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	btns := dv.ContextMenuBtns()
	if len(btns) == 0 {
		t.Fatal("no context menu buttons")
	}

	// Find "Rename" button (present on all platforms).
	var renameBtn *Button
	for _, btn := range btns {
		if btn.Text == "Rename" {
			renameBtn = btn
			break
		}
	}
	if renameBtn == nil {
		t.Skip("Rename button not found")
	}
	r := renameBtn.Rect()
	if r.Empty() {
		t.Skip("Rename button rect empty")
	}
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	// Ensure the button is inside the scroll viewport (visible).
	scroll := dv.ContextMenuScrollForTest()
	if scroll != nil && !image.Pt(tx, ty).In(scroll.VS.View) {
		t.Skip("Rename button not in viewport")
	}

	// Press.
	dv.handleContextMenuInput(tx, ty, true)
	// Release at same position — should fire the tap.
	dv.handleContextMenuInput(tx, ty, false)

	// Rename fires onClick which sets contextMenuOpen = false.
	if dv.ContextMenuOpen() {
		t.Error("expected context menu to close after tapping Rename button")
	}
}

// TestContextMenuDeleteReachableViaScroll verifies that scrolling to the bottom
// makes the "Delete" button visible within the menu rect.
func TestContextMenuDeleteReachableViaScroll(t *testing.T) {
	dv := newSmallDrumView(t, 390, 180)
	dv.OpenContextMenu(0)

	scroll := dv.ContextMenuScrollForTest()
	if scroll == nil || !scroll.HasScroll() {
		t.Skip("no scroll needed — Delete may already be visible")
	}

	// Scroll to bottom.
	scroll.VS.First = scroll.VS.Total - scroll.VS.Visible
	scroll.VS.Clamp()
	dv.rebuildContextMenuButtons()

	// Find the Delete button.
	btns := dv.ContextMenuBtns()
	var deleteBtn *Button
	for _, btn := range btns {
		if btn.Text == "Delete" {
			deleteBtn = btn
			break
		}
	}
	if deleteBtn == nil {
		t.Fatal("Delete button not found in context menu buttons")
	}

	menuRect := dv.ContextMenuRectVal()
	r := deleteBtn.Rect()
	if r.Empty() {
		t.Fatal("Delete button rect is empty")
	}

	// Check that at least part of the Delete button is within the menu rect.
	if !r.Overlaps(menuRect) {
		t.Errorf("Delete button (%v) not within menu rect (%v) after scrolling to bottom", r, menuRect)
	}
}

// TestContextMenuScrollMomentum verifies that releasing a fast swipe creates
// momentum that continues scrolling.
func TestContextMenuScrollMomentum(t *testing.T) {
	// 130px keeps Visible=1 with 4 mobile items, leaving room for momentum
	// to advance VS.First by more than one step.
	dv := newSmallDrumView(t, 390, 130)
	dv.OpenContextMenu(0)

	scroll := dv.ContextMenuScrollForTest()
	if scroll == nil || !scroll.HasScroll() {
		t.Skip("no scroll needed on this screen size")
	}

	// Simulate a fast swipe.
	itemView := scroll.VS.View
	sx := itemView.Min.X + itemView.Dx()/2
	sy := itemView.Min.Y + 40

	scroll.HandleTouchBegin(sx, sy)
	// Multiple fast moves.
	scroll.HandleTouchMove(sx, sy-30)
	scroll.HandleTouchMove(sx, sy-60)
	scroll.HandleTouchMove(sx, sy-90)
	scroll.HandleTouchEnd()

	if !scroll.HasMomentum() {
		t.Error("expected HasMomentum()=true after fast swipe")
	}

	firstBefore := scroll.VS.First
	// Simulate momentum updates.
	for i := 0; i < 10; i++ {
		scroll.UpdateMomentum()
	}

	if scroll.VS.First <= firstBefore {
		t.Errorf("expected VS.First to increase during momentum; before=%d after=%d",
			firstBefore, scroll.VS.First)
	}
}
