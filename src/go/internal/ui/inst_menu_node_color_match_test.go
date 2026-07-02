//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestInstMenuInstrumentRowMatchesNodeColor verifies the Categories→Instrument
// navigation entry for an instrument that lives on a row renders in that row's
// (node's) actual color, not the instrument's registered default. Reproduces the
// bug where a user-recolored node showed a mismatched swatch/stripe in the
// picker because the menu derived its color from instColor(id) (the global
// instrument-default lookup) instead of the live DrumRow.Color.
func TestInstMenuInstrumentRowMatchesNodeColor(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.instOptions = []string{"kick", "snare"}
	// NewDrumView seeds a default row 0; bind it to the kick instrument so the
	// menu opens with kick as the current/active instrument.
	dv.Rows[0].Instrument = "kick"
	dv.calcLayout()

	// Recolor the node to a warm orange that deliberately differs from the
	// kick instrument's registered default — this is the value the grid node
	// is drawn with (game_draw_grid_pane.go reads dv.Rows[i].Color).
	want := color.RGBA{190, 120, 60, 255}
	def := color.RGBAModel.Convert(instColor("kick")).(color.RGBA)
	if want == def {
		t.Skip("warm fixture coincides with kick default; pick a different fixture")
	}
	dv.Rows[0].Color = want

	comp := NewInstrumentMenuComponent()
	dv.instMenuComp = comp
	dv.openInstMenuForRow(0)

	// Drill into the instrument list so the kick row (the active instrument) is
	// drawn with its active stripe + swatch.
	comp.state.mode = InstMenuModeInstruments
	comp.rebuildMenu()

	img := ebiten.NewImage(800, 600)
	rects := collectFilledRects(t, func() { comp.Draw(img) })

	// The active instrument row draws a thin, full-row-height accent stripe at
	// its left edge in the instrument's color. It must be the node color.
	foundNodeColor := false
	foundStaleDefault := false
	for _, dr := range rects {
		if dr.Rect.Dx() <= accentStripeW()+1 && dr.Rect.Dy() >= 12 {
			if dr.Color == want {
				foundNodeColor = true
			}
			if dr.Color == def {
				foundStaleDefault = true
			}
		}
	}
	if !foundNodeColor {
		t.Errorf("kick instrument row stripe should tint to the node color %v; rects=%v", want, rects)
	}
	if foundStaleDefault {
		t.Errorf("kick instrument row stripe still shows the stale instrument default %v instead of the node color", def)
	}
}

// TestInstMenuInstrumentRowFollowsLiveNodeRecolor verifies that recoloring a
// node WHILE the instrument menu is open updates the menu's swatch/stripe to the
// new color (SetRowColorManual refreshes the open menu's derived colors).
func TestInstMenuInstrumentRowFollowsLiveNodeRecolor(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.instOptions = []string{"kick", "snare"}
	dv.Rows[0].Instrument = "kick"
	dv.calcLayout()

	comp := NewInstrumentMenuComponent()
	dv.instMenuComp = comp
	dv.openInstMenuForRow(0)
	comp.state.mode = InstMenuModeInstruments
	comp.rebuildMenu()

	// Recolor the node while the menu is open.
	want := color.RGBA{60, 190, 120, 255} // teal, distinct from any default
	def := color.RGBAModel.Convert(instColor("kick")).(color.RGBA)
	if want == def {
		t.Skip("teal fixture coincides with kick default; pick a different fixture")
	}
	dv.SetRowColorManual(0, want)

	img := ebiten.NewImage(800, 600)
	rects := collectFilledRects(t, func() { comp.Draw(img) })

	found := false
	for _, dr := range rects {
		if dr.Rect.Dx() <= accentStripeW()+1 && dr.Rect.Dy() >= 12 && dr.Color == want {
			found = true
		}
	}
	if !found {
		t.Errorf("after live recolor, kick row stripe should follow the new node color %v; rects=%v", want, rects)
	}
}
