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

// Every mobile bottom sheet carries the same drag-handle pill. The File
// (overflow) sheet was the odd one out — context menu and instrument picker
// drew it, File didn't (bottom-sheet chrome inconsistency, 2026-07-04
// critique D-item).
func TestOverflowSheetHasDragHandleOnMobile(t *testing.T) {
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
	if !hasSheetHandlePill(t, dv.overflowPopupRect(), func() { dv.drawOverflowMenu(dst) }) {
		t.Fatal("mobile File sheet drew no drag-handle pill — sheet chrome must match the context menu and instrument picker")
	}
}
