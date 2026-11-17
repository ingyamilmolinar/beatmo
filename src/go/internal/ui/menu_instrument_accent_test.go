//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestContextMenuAccentIsInstrumentColor verifies the per-row context (kebab)
// menu reports the row's instrument color as its accent, so its chrome
// (header underline + active item stripe) tints to the instrument.
func TestContextMenuAccentIsInstrumentColor(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	want := color.RGBA{190, 120, 60, 255}
	dv.Rows[0].Color = want
	dv.OpenContextMenu(0)

	got := dv.contextMenuAccent()
	if !colorsEqual(got, want) {
		t.Errorf("contextMenuAccent() = %v, want instrument color %v", got, want)
	}
}

// TestContextMenuHeaderTintsToInstrument verifies the mobile context-menu
// header underline is rendered in the instrument color family (warm: R≥B),
// not the fixed azure (B≫R), proving the accent is wired into the draw path.
func TestContextMenuHeaderTintsToInstrument(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	dv := NewDrumView(image.Rect(0, 0, 390, 700), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	dv.Rows[0].Color = color.RGBA{190, 120, 60, 255} // warm orange
	dv.OpenContextMenu(0)

	img := ebiten.NewImage(390, 700)
	rects := collectFilledRects(t, func() { dv.drawContextMenu(img) })

	warmUnderline := false
	for _, dr := range rects {
		// The 1px header underline tinted by the accent.
		if dr.Rect.Dy() == 1 && dr.Color.A > 0 && dr.Color.R >= dr.Color.B && dr.Color.R > 0 {
			warmUnderline = true
		}
	}
	if !warmUnderline {
		t.Errorf("context menu header underline should tint to the warm instrument color; rects=%v", rects)
	}
}
