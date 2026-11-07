//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The picker takes the live row colour, not the instrument's registered default, or a recoloured node shows a
// mismatched swatch.
func TestInstMenuInstrumentRowMatchesNodeColor(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.instOptions = []instrument.ID{roster.Kick, roster.Snare}
	dv.Rows[0].Instrument = roster.Kick
	dv.calcLayout()

	// Differs from kick's default: this is the colour the grid node is drawn with.
	want := color.RGBA{190, 120, 60, 255}
	def := color.RGBAModel.Convert(instColor(roster.Kick)).(color.RGBA)
	if want == def {
		t.Skip("warm fixture coincides with kick default; pick a different fixture")
	}
	dv.Rows[0].Color = want

	comp := NewInstrumentMenuComponent()
	dv.instMenuComp = comp
	dv.openInstMenuForRow(0)

	comp.state.mode = InstMenuModeInstruments
	comp.rebuildMenu()

	img := ebiten.NewImage(800, 600)
	rects := collectFilledRects(t, func() { comp.Draw(img) })

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

func TestInstMenuInstrumentRowFollowsLiveNodeRecolor(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.instOptions = []instrument.ID{roster.Kick, roster.Snare}
	dv.Rows[0].Instrument = roster.Kick
	dv.calcLayout()

	comp := NewInstrumentMenuComponent()
	dv.instMenuComp = comp
	dv.openInstMenuForRow(0)
	comp.state.mode = InstMenuModeInstruments
	comp.rebuildMenu()

	// Recolor the node while the menu is open.
	want := color.RGBA{60, 190, 120, 255} // teal, distinct from any default
	def := color.RGBAModel.Convert(instColor(roster.Kick)).(color.RGBA)
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
