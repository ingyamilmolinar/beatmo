//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The active stripe takes the instrument's own colour so the highlight matches its swatch.
func TestInstrumentMenuActiveRowTintsToInstrumentColor(t *testing.T) {
	assertDefaultParityState(t)

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 200, 124),
		VertBounds:        image.Rect(0, 50, 400, 700),
		CurrentInstrument: roster.Kick,
		Categories:        []audio.Category{audio.CatKick},
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatKick},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatKick},
		},
		RowHeight:     28,
		LabelWidth:    120,
		ControlsWidth: 200,
	})
	comp.Open()
	comp.state.mode = InstMenuModeInstruments
	comp.rebuildMenu()

	want := color.RGBAModel.Convert(instColor(roster.Kick)).(color.RGBA)
	azure := color.RGBAModel.Convert(colAccent).(color.RGBA)
	if want == azure {
		t.Skip("kick color coincides with azure; pick a different fixture")
	}

	img := ebiten.NewImage(400, 700)
	rects := collectFilledRects(t, func() { comp.Draw(img) })

	// A thin full-row-height bar; the height excludes the swatch's rounded-corner slivers.
	found := false
	for _, dr := range rects {
		if dr.Color == want && dr.Rect.Dx() <= accentStripeW()+1 && dr.Rect.Dy() >= 12 {
			found = true
		}
	}
	if !found {
		t.Errorf("selected instrument row stripe should tint to instColor(kick)=%v; rects=%v", want, rects)
	}
}
