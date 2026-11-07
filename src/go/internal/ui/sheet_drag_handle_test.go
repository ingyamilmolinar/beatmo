//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

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

// The File menu is a floating card in the grid pane, not a bottom sheet, and cards carry no drag handle; on a card
// it reads as a stray line. True bottom sheets keep theirs.
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
