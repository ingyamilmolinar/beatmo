//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// hasSheetHandlePill reports whether fn draws the standard 36×4 drag-handle
// pill near the top of sheetRect.
func hasSheetHandlePill(t *testing.T, sheetRect image.Rectangle, fn func()) bool {
	t.Helper()
	origRR := drawRoundedRect
	defer func() { drawRoundedRect = origRR }()

	var found bool
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if filled && r.Dx() == 36 && r.Dy() == 4 &&
			r.Min.Y >= sheetRect.Min.Y && r.Min.Y <= sheetRect.Min.Y+16 {
			found = true
		}
		origRR(dst, r, c, radius, filled)
	}
	fn()
	return found
}

// The File (overflow) menu is NOT a bottom sheet: on every screen size it
// anchors as a floating card in the grid pane, exactly like the node pop-up,
// Settings, and Logs cards. Those cards carry no drag handle, so neither does
// File — the handle read as a stray horizontal line at the top of the card
// (2026-07-22 card-consistency pass). This asserts the handle is gone on
// mobile; the context menu and instrument picker (true bottom sheets) keep
// their handle via drawSheetDragHandle.
func TestOverflowMenuHasNoDragHandleOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	dv := g.drum
	dv.OpenOverflowMenu()
	dv.rebuildOverflowBtns()

	dst := ebiten.NewImage(390, 844)
	if hasSheetHandlePill(t, dv.overflowPopupRect(), func() { dv.drawOverflowMenu(dst) }) {
		t.Fatal("mobile File card drew a drag-handle pill — it is a floating grid-pane card, not a bottom sheet; must match Settings/Logs (no handle)")
	}
}
