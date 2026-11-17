//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// setEQPanelCursorTestInput replaces cursorPosition/isMouseButtonPressed with
// fixed values for the duration of the test. mx/my anchor the desktop cursor;
// pressed gates isMouseButtonPressed(MouseButtonLeft).
func setEQPanelCursorTestInput(t *testing.T, mx, my int, pressed bool) func() {
	t.Helper()
	return SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
}

// laidOutEQZoneForCursor returns an EQPanelZone wired into a tree at a known
// rect and forces the requested active tab. The zone's stickyBar is created
// (so freeze etc. don't NPE inside Draw). Pass mobile=true to register the
// touch scrub hit area before the second rebuild.
func laidOutEQZoneForCursor(t *testing.T, tab PanelTab) *EQPanelZone {
	t.Helper()
	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))
	restore := noInputForTest()
	tree.Update()
	restore()
	z.tabState.SetActiveTab(tab)
	// Re-run hit-area build so any tab-conditional areas reflect the new tab.
	z.rebuildHitAreas()
	return z
}

// TestCursorHoverDesktopUpdatesPosition — on desktop, moving the cursor into
// the content rect on the Spectrum tab sets cursorX and cursorActive.
func TestCursorHoverDesktopUpdatesPosition(t *testing.T) {
	assertDefaultParityState(t)
	// Desktop profile is the default — explicit reset for clarity.
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z := laidOutEQZoneForCursor(t, TabSpectrum)
	cr := z.bodyRect()
	if cr.Empty() {
		t.Fatal("content rect should be non-empty")
	}
	wantX := cr.Min.X + 100
	if wantX >= cr.Max.X {
		wantX = (cr.Min.X + cr.Max.X) / 2
	}
	wantY := cr.Min.Y + 10
	restore := setEQPanelCursorTestInput(t, wantX, wantY, false)
	defer restore()

	z.updateAudioPanelCursor()

	if !z.cursorActive {
		t.Errorf("cursorActive=false; want true with mouse inside content rect")
	}
	if z.cursorX != wantX {
		t.Errorf("cursorX=%d, want %d", z.cursorX, wantX)
	}
}

// TestCursorExitDesktopClears — when the mouse leaves the content rect on
// desktop, cursorActive must drop back to false.
func TestCursorExitDesktopClears(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z := laidOutEQZoneForCursor(t, TabWave)
	cr := z.bodyRect()
	if cr.Empty() {
		t.Fatal("content rect should be non-empty")
	}
	// First enter.
	restoreIn := setEQPanelCursorTestInput(t, cr.Min.X+50, cr.Min.Y+10, false)
	z.updateAudioPanelCursor()
	restoreIn()
	if !z.cursorActive {
		t.Fatal("precondition: cursorActive should be true while hovering")
	}
	// Now leave the rect (move above it).
	restoreOut := setEQPanelCursorTestInput(t, cr.Min.X-100, cr.Min.Y-100, false)
	defer restoreOut()
	z.updateAudioPanelCursor()
	if z.cursorActive {
		t.Errorf("cursorActive=true; want false after exit")
	}
}

// TestCursorTouchDragUpdatesPosition — on mobile, a press+drag inside the
// content rect drives cursorX via the scrub handler.
func TestCursorTouchDragUpdatesPosition(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z := laidOutEQZoneForCursor(t, TabSpectrum)
	cr := z.bodyRect()
	if cr.Empty() {
		t.Fatal("content rect should be non-empty")
	}
	pressX := cr.Min.X + 30
	pressY := cr.Min.Y + 10
	dragX := cr.Min.X + 80

	// Drive the handler directly — same code path the tree's HitIndex would
	// invoke for the registered "eq-audio-cursor-scrub" hit area.
	h := &cursorScrubHandler{zone: z}
	if got := h.OnPress(pressX, pressY); got != InputCaptured {
		t.Errorf("OnPress returned %v, want InputCaptured", got)
	}
	if !z.cursorActive {
		t.Errorf("cursorActive=false after press; want true")
	}
	if z.cursorX != pressX {
		t.Errorf("cursorX after press=%d, want %d", z.cursorX, pressX)
	}
	h.OnDrag(dragX, pressY)
	if z.cursorX != dragX {
		t.Errorf("cursorX after drag=%d, want %d", z.cursorX, dragX)
	}
}

// TestCursorTouchReleasePersists — after release the cursor stays visible
// (latched via cursorPinned).
func TestCursorTouchReleasePersists(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z := laidOutEQZoneForCursor(t, TabWave)
	cr := z.bodyRect()
	if cr.Empty() {
		t.Fatal("content rect should be non-empty")
	}
	pressX := cr.Min.X + 40
	h := &cursorScrubHandler{zone: z}
	h.OnPress(pressX, cr.Min.Y+10)
	h.OnRelease(pressX, cr.Min.Y+10)
	if !z.cursorPinned {
		t.Errorf("cursorPinned=false after release; want true (latched)")
	}
	// After Update, mobile path must NOT clear pinned state — there's no
	// per-frame mobile reset.
	restore := setEQPanelCursorTestInput(t, 0, 0, false)
	defer restore()
	z.updateAudioPanelCursor()
	if !z.cursorPinned {
		t.Errorf("cursorPinned=false after subsequent Update; want true (still latched)")
	}
}

// TestCursorActiveOnlyOnAnalysisTabs — switching to TabEQ clears cursor state.
func TestCursorActiveOnlyOnAnalysisTabs(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z := laidOutEQZoneForCursor(t, TabSpectrum)
	cr := z.bodyRect()
	if cr.Empty() {
		t.Fatal("content rect should be non-empty")
	}
	// Activate on Spectrum.
	restore := setEQPanelCursorTestInput(t, cr.Min.X+50, cr.Min.Y+10, false)
	z.updateAudioPanelCursor()
	if !z.cursorActive {
		t.Fatal("precondition: cursorActive=true on Spectrum")
	}
	restore()

	// Switch to EQ → updateAudioPanelCursor must clear.
	z.tabState.SetActiveTab(TabEQ)
	z.cursorPinned = true // simulate prior pinned state to ensure it's cleared
	restore2 := setEQPanelCursorTestInput(t, cr.Min.X+50, cr.Min.Y+10, false)
	defer restore2()
	z.updateAudioPanelCursor()
	if z.cursorActive {
		t.Errorf("cursorActive=true on TabEQ; want false")
	}
	if z.cursorPinned {
		t.Errorf("cursorPinned=true on TabEQ; want false")
	}
}
