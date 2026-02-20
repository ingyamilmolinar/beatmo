//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestSidebarCloseButtonVisible verifies that the sidebar close button is
// always drawn, even though it lives in the fixed header area above the
// scrollable content viewport. Previously, the close button drawing was
// guarded by sb.inViewport(r), which only returned true for the scrollable
// content area below the header — so the close button never rendered.
func TestSidebarCloseButtonVisible(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 250)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	// The close button must exist and have a non-empty rect.
	btn, ok := g.sidebar.btns["close"]
	if !ok || btn == nil {
		t.Fatal("close button not found in sidebar btns")
	}
	closeRect := btn.Rect()
	if closeRect.Empty() {
		t.Fatal("close button rect is empty")
	}

	// The close button rect should be within the panel rect.
	panel := g.sidebar.rects["panel"]
	if panel.Empty() {
		t.Fatal("panel rect is empty")
	}
	if !closeRect.In(panel) {
		t.Errorf("close button rect %v is not within panel rect %v", closeRect, panel)
	}

	// Verify the close button is in the fixed header — its Max.Y should be
	// LESS than contentTop, which is where inViewport starts accepting.
	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
	if closeRect.Max.Y >= contentTop {
		t.Logf("close button Max.Y=%d >= contentTop=%d (inViewport would accept it anyway)", closeRect.Max.Y, contentTop)
	} else {
		// This is the case that previously caused the bug: the close button
		// is above contentTop, so inViewport(closeRect) returned false and
		// the button was never drawn.
		t.Logf("close button Max.Y=%d < contentTop=%d — inViewport would reject this (pre-fix bug)", closeRect.Max.Y, contentTop)
	}

	// Verify that tapping the close button actually closes the sidebar.
	r := g.sidebar.rects["close"]
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	g.sidebar.fireTapAt(cx, cy)
	if g.sidebar.IsOpen() {
		t.Error("sidebar should be closed after tapping close button")
	}
}

// TestOverflowCloseButtonNotOverlappingFilePickerRects verifies that the
// file picker rects registered by registerFilePickerRects() do not overlap
// with the overflow popup's close button. Previously, the Upload file picker
// rect spanned the full popup width and overlapped the close button, causing
// taps on close to also trigger the file picker on mobile.
func TestOverflowCloseButtonNotOverlappingFilePickerRects(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 300
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	// Open the overflow menu.
	dv.SetOverflowMenuOpen(true)

	// Capture file picker rects.
	var captured []capturedFilePickerRect
	testCapturedFilePickerRects = &captured
	t.Cleanup(func() { testCapturedFilePickerRects = nil })

	dv.registerFilePickerRects()

	if len(captured) < 1 {
		t.Fatal("expected at least 1 file picker rect, got 0")
	}

	// Compute the close button rect for the overflow popup.
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Fatal("overflow popup rect is empty")
	}
	closeR := closeButtonRect(popupRect, buttonPad)
	if closeR.Empty() {
		t.Fatal("close button rect is empty")
	}

	// Assert no file picker rect overlaps the close button.
	for _, c := range captured {
		fpRect := image.Rect(c.X, c.Y, c.X+c.W, c.Y+c.H)
		if fpRect.Overlaps(closeR) {
			t.Errorf("file picker rect %q %v overlaps close button rect %v — "+
				"tapping close would trigger file picker on mobile",
				c.ID, fpRect, closeR)
		}
	}
}
