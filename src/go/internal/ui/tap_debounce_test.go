//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Through the real touch pipeline, in the row-scroll dead zone: the one region where a tap is delivered by
// injection. Elsewhere the press dispatches live and there is no injection cycle to debounce.
func TestMobileTapDebounceDropsSameSpotDoubleTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(414, 896)
	g.Update() // build demo + lay out rows so RowsRect is populated
	globalTouchState.Reset()
	t.Cleanup(globalTouchState.Reset)
	t.Cleanup(resetTouchOverride)

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// A point inside the mobile row-scroll dead zone (rows view, not EQ mode).
	rr := g.drum.RowsRect()
	if rr.Empty() || g.drum.MobileEQMode() {
		t.Skip("row-scroll dead zone not available at this layout")
	}
	px, py := (rr.Min.X+rr.Max.X)/2, (rr.Min.Y+rr.Max.Y)/2
	if g.split.InGridPane(px, py) {
		t.Skipf("dead-zone point (%d,%d) is in the grid pane", px, py)
	}

	tap := func(id ebiten.TouchID) {
		mock.addTouch(id, px, py)
		g.Update() // touch down
		g.Update() // hold a frame (dead zone blocks live dispatch)
		mock.removeTouch(id)
		g.Update() // touch up → GestureTap → injectTouchTap
	}

	// Tap 1 fires → an injection cycle starts.
	tap(1)
	if !isTouchTapInjecting() {
		t.Fatal("first drum-area tap must start a tap injection")
	}
	// Let tap 1's 2-frame injection cycle finish.
	g.Update()
	g.Update()
	if isTouchTapInjecting() {
		t.Fatal("tap 1 injection should have cleared before the second tap")
	}

	// Tap 2: same spot, within the window → dropped, no injection.
	tap(2)
	if isTouchTapInjecting() {
		t.Fatal("a same-spot re-tap within the debounce window must be dropped (no injection)")
	}

	// After the window elapses, the same spot is a fresh tap again.
	for i := 0; i < tapDebounceFrames+2; i++ {
		g.Update()
	}
	tap(3)
	if !isTouchTapInjecting() {
		t.Fatal("a same-spot tap after the debounce window must inject again")
	}
}

// Touch-bounce or an accidental double-tap at the same point within the window is rejected.
func TestTapDebounceRejectsSameSpotBurst(t *testing.T) {
	var d tapDebounce

	// First tap always fires.
	if !d.accept(100, 100) {
		t.Fatal("first tap must be accepted")
	}

	// Immediate re-tap at the same point (bounce) — rejected.
	if d.accept(100, 100) {
		t.Fatal("same-spot re-tap within the window must be rejected (bounce)")
	}

	// Within-slop jitter still counts as the same tap — rejected.
	if d.accept(100+tapDebounceSlopPx-1, 100) {
		t.Fatal("within-slop re-tap within the window must be rejected")
	}

	// A tap clearly on a different control (beyond slop) fires even in-window.
	if !d.accept(100+tapDebounceSlopPx*4, 100) {
		t.Fatal("a tap beyond the slop is a different control — must be accepted")
	}
}

func TestTapDebounceAcceptsAfterWindow(t *testing.T) {
	var d tapDebounce

	if !d.accept(50, 50) {
		t.Fatal("first tap must be accepted")
	}
	if d.accept(50, 50) {
		t.Fatal("same-spot re-tap within the window must be rejected")
	}

	// Advance past the window.
	for i := 0; i < tapDebounceFrames; i++ {
		d.advance()
	}
	if !d.accept(50, 50) {
		t.Fatal("same-spot tap after the window elapses must be accepted")
	}
}
