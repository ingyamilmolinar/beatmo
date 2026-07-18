//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// fx_panel_close_reopen_test.go — "one press belongs to ONE subtree for its
// whole lifetime" contract.
//
// A real mouse click holds the button down for many frames. The RootTree routes
// each frame's input to exactly one subtree and WITHHOLDS the press from lower
// subtrees while an upper one is busy — but a withheld subtree must also stay
// withheld for the REST of that press, not just the frame the upper subtree
// happened to be busy on.
//
// The FX panel is the reported repro: its close "X" is painted directly over the
// NEXT row's FX button (row 0's panel close sits inside row 1's fxBtn rect). The
// close button lives in the overlay subtree's portal (z >= 300); the row's FX
// button lives in the drum-view subtree's row rack (z = 120). Pressing X closes
// the panel on the press-edge frame; on the FOLLOWING held frame the overlay
// subtree is no longer busy (its portal is empty and Tick reset inputHandled),
// so the drum-view subtree — which never latched this press cycle while it was
// withheld — reads the still-held pointer as a FRESH press edge and dispatches
// it to the FX button underneath, re-opening the panel for the other row.

// injectHeldPressAt holds a LEFT-button press at (x,y) for `frames` consecutive
// frames through the production input seams, then releases it — i.e. a REAL
// click, which never lasts a single frame. The existing injectPressAt holds for
// exactly one frame, which is why no test caught a press leaking to a lower
// subtree on the SECOND held frame.
func injectHeldPressAt(t *testing.T, dv *DrumView, x, y, frames int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 900 },
	)
	for i := 0; i < frames; i++ {
		dv.Update()
	}
	restore()
	injectRelease(t, dv, x, y)
}

// fxCloseCenterOverRow opens row 0's FX panel and returns the centre of its
// close "X" plus the index of the OTHER row whose FX button sits underneath that
// point. It fails the test if the panel's close button does not overlap a
// different row's FX button — that overlap is the repro's precondition.
func fxCloseCenterOverRow(t *testing.T, dv *DrumView) (image.Point, int) {
	t.Helper()

	dv.OpenFXPanel(0)
	dv.layoutForTest()

	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open for row 0")
	}
	closeB := dv.fxPanelCloseBtn()
	if closeB == nil {
		t.Fatal("FX panel has no close button")
	}
	cr := closeB.Rect()
	if cr.Empty() {
		t.Fatal("FX panel close button has an empty rect")
	}
	c := image.Pt(cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2)

	for i, b := range dv.rowFXBtns() {
		if i == 0 || b == nil {
			continue // row 0 owns the open panel; we want a DIFFERENT row
		}
		if c.In(b.Rect()) {
			return c, i
		}
	}
	t.Fatalf("close button %v does not overlap any other row's FX button — "+
		"repro precondition unmet (panel rect %v)", cr, dv.fxPanelRect)
	return image.Point{}, -1
}

// TestFXPanelCloseButtonDoesNotReopenPanelForRowBeneath is the reported bug:
// clicking the FX panel's "X" closes the panel and IMMEDIATELY re-opens it for
// the row whose FX button is painted beneath the X.
func TestFXPanelCloseButtonDoesNotReopenPanelForRowBeneath(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()

	c, beneath := fxCloseCenterOverRow(t, dv)

	// A real click: the button stays down for several frames.
	injectHeldPressAt(t, dv, c.X, c.Y, 4)

	if dv.IsFXPanelOpen() {
		t.Fatalf("clicking the FX panel close button at %v closed the panel and then "+
			"re-opened it for row %d (fxPanelRow=%d): the press was consumed by the "+
			"overlay subtree's close button, but the drum-view subtree re-read the "+
			"still-held pointer as a fresh press and dispatched it to the FX button "+
			"underneath. One press must belong to ONE subtree for its whole lifetime.",
			c, beneath, dv.fxPanelRow)
	}
}

// TestFXPanelOpenViaButtonThenCloseViaXStaysClosed drives the user's exact
// desktop flow end-to-end with REAL held clicks: click row 0's FX button to open
// the panel, then click the panel's "X" to close it. Nothing may re-open.
func TestFXPanelOpenViaButtonThenCloseViaXStaysClosed(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()

	fxBtns := dv.rowFXBtns()
	if len(fxBtns) == 0 || fxBtns[0] == nil {
		t.Fatal("row 0 has no FX button")
	}
	br := fxBtns[0].Rect()
	if br.Empty() {
		t.Fatal("row 0 FX button has an empty rect")
	}

	// Open by clicking row 0's FX button, as a user does.
	injectHeldPressAt(t, dv, br.Min.X+br.Dx()/2, br.Min.Y+br.Dy()/2, 4)
	dv.layoutForTest()
	if !dv.IsFXPanelOpen() {
		t.Fatal("clicking row 0's FX button did not open the FX panel")
	}
	if dv.fxPanelRow != 0 {
		t.Fatalf("FX panel opened for row %d, want row 0", dv.fxPanelRow)
	}

	closeB := dv.fxPanelCloseBtn()
	if closeB == nil {
		t.Fatal("FX panel has no close button")
	}
	cr := closeB.Rect()
	cx, cy := cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2

	// Close by clicking the "X", as a user does.
	injectHeldPressAt(t, dv, cx, cy, 4)

	if dv.IsFXPanelOpen() {
		t.Fatalf("after clicking the FX panel's X at (%d,%d) the panel is still open "+
			"for row %d — the close leaked through to the control beneath the X",
			cx, cy, dv.fxPanelRow)
	}
}

// TestFXPanelCloseButtonClosesOnFirstHeldFrame guards the other half: the close
// must actually happen on the press edge (and stay closed), so a fix that merely
// suppresses the re-open cannot regress the close itself.
func TestFXPanelCloseButtonClosesOnFirstHeldFrame(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()

	c, _ := fxCloseCenterOverRow(t, dv)

	restore := SetInputForTest(
		func() (int, int) { return c.X, c.Y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 900 },
	)
	dv.Update()
	closedOnPressEdge := !dv.IsFXPanelOpen()
	restore()
	injectRelease(t, dv, c.X, c.Y)

	if !closedOnPressEdge {
		t.Fatal("FX panel close button did not close the panel on the press edge")
	}
	if dv.IsFXPanelOpen() {
		t.Fatalf("FX panel re-opened after release (fxPanelRow=%d)", dv.fxPanelRow)
	}
}
