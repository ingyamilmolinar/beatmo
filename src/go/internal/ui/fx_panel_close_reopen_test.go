//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// One press belongs to ONE tree for its whole life. The FX panel's close × sits over the next row's FX chip:
// closing on the press edge empties the overlay, and the still-held pointer must not then read as a fresh
// press in the drum tree and reopen the panel for that row.

// A real click spans many frames; a one-frame press never shows the leak on the second held frame.
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

// The panel spans the rack column, so its × lands over another row's right-most control (FX chip or ⋯); fails
// without that overlap, the repro's precondition.
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

	for _, btns := range [][]*Button{dv.rowFXBtns(), dv.rowRackZone.RowMenuBtns()} {
		for i, b := range btns {
			if i == 0 || b == nil {
				continue // row 0 owns the open panel; we want a DIFFERENT row
			}
			if c.In(b.Rect()) {
				return c, i
			}
		}
	}
	t.Fatalf("close button %v does not overlap any other row's FX or kebab button — "+
		"repro precondition unmet (panel rect %v)", cr, dv.fxPanelRect)
	return image.Point{}, -1
}

func TestFXPanelCloseButtonDoesNotReopenPanelForRowBeneath(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()

	c, beneath := fxCloseCenterOverRow(t, dv)

	// A real click: the button stays down for several frames.
	injectHeldPressAt(t, dv, c.X, c.Y, 4)

	if dv.IsContextMenuOpen() {
		t.Fatalf("clicking the FX panel close button at %v opened row %d's context "+
			"menu: the press leaked to the kebab beneath the X", c, beneath)
	}
	if dv.IsFXPanelOpen() {
		t.Fatalf("clicking the FX panel close button at %v closed the panel and then "+
			"re-opened it for row %d (fxPanelRow=%d): the press was consumed by the "+
			"overlay subtree's close button, but the drum-view subtree re-read the "+
			"still-held pointer as a fresh press and dispatched it to the FX button "+
			"underneath. One press must belong to ONE subtree for its whole lifetime.",
			c, beneath, dv.fxPanelRow)
	}
}

// The desktop flow with real held clicks: open from row 0's FX, close with ×, nothing reopens.
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

// The close happens on the press edge, so a fix that only suppresses the reopen cannot break it.
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
	if dv.IsContextMenuOpen() {
		t.Fatal("the close press leaked to the kebab beneath the X (context menu opened)")
	}
}
