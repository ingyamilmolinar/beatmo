//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Drawn although it sits in the fixed header above the scroll viewport, which inViewport alone rejects.
func TestSidebarCloseButtonVisible(t *testing.T) {
	assertDefaultParityState(t)
	// The close × is desktop only; the mobile card dismisses by tap-away.
	withSmallScreen(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 250)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	// The close button must exist and have a non-empty rect.
	btn, ok := g.sidebar.btns[sbClose]
	if !ok || btn == nil {
		t.Fatal("close button not found in sidebar btns")
	}
	closeRect := btn.Rect()
	if closeRect.Empty() {
		t.Fatal("close button rect is empty")
	}

	// The close button rect should be within the panel rect.
	panel := g.sidebar.rects[sbPanel]
	if panel.Empty() {
		t.Fatal("panel rect is empty")
	}
	if !closeRect.In(panel) {
		t.Errorf("close button rect %v is not within panel rect %v", closeRect, panel)
	}

	// In the fixed header, above where inViewport starts accepting.
	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
	if closeRect.Max.Y >= contentTop {
		t.Logf("close button Max.Y=%d >= contentTop=%d (inViewport would accept it anyway)", closeRect.Max.Y, contentTop)
	} else {
		t.Logf("close button Max.Y=%d < contentTop=%d — inViewport would reject this (pre-fix bug)", closeRect.Max.Y, contentTop)
	}

	// Verify that tapping the close button actually closes the sidebar.
	r := g.sidebar.rects[sbClose]
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	g.sidebar.fireTapAt(cx, cy)
	if g.sidebar.IsOpen() {
		t.Error("sidebar should be closed after tapping close button")
	}
}

// A full-width Upload rect over the close button would make a tap on close also open the picker on mobile.
func TestOverflowCloseButtonNotOverlappingFilePickerRects(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 300
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	// Open the overflow menu (File page).
	dv.SetOverflowMenuOpen(true)
	dv.overflowPage = 0

	captured := dv.filePickerCandidates()

	if len(captured) < 1 {
		t.Fatal("expected at least 1 file picker rect, got 0")
	}

	// Compute the close button rect for the overflow popup.
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Fatal("overflow popup rect is empty")
	}
	closeR := closeButtonRect(popupRect, SpaceXS)
	if closeR.Empty() {
		t.Fatal("close button rect is empty")
	}

	// Assert no file picker rect overlaps the close button.
	for _, c := range captured {
		if c.Rect.Overlaps(closeR) {
			t.Errorf("file picker rect %q %v overlaps close button rect %v — "+
				"tapping close would trigger file picker on mobile",
				c.Intent.ID, c.Rect, closeR)
		}
	}
}
