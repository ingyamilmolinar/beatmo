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

// The picker's chrome takes its owning instrument's colour, not the fixed azure.
func TestInstMenuCategoryActiveTintsToCurrentInstrument(t *testing.T) {
	assertDefaultParityState(t)

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 200, 124),
		VertBounds:        image.Rect(0, 50, 400, 700),
		CurrentInstrument: roster.Kick,
		Categories:        []audio.Category{audio.CatKick, audio.CatSnare},
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatKick},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatSnare},
		},
		RowHeight:       28,
		LabelWidth:      120,
		ControlsWidth:   200,
		ForceCategories: true,
	})
	comp.Open()
	comp.state.activeCat = audio.CatKick
	comp.state.mode = InstMenuModeCategories
	comp.rebuildMenu()

	want := color.RGBAModel.Convert(instColor(roster.Kick)).(color.RGBA)
	azure := color.RGBAModel.Convert(colAccent).(color.RGBA)
	if want == azure {
		t.Skip("kick color coincides with azure")
	}

	img := ebiten.NewImage(400, 700)
	rects := collectFilledRects(t, func() { comp.Draw(img) })

	found := false
	for _, dr := range rects {
		if dr.Color == want && dr.Rect.Dx() <= accentStripeW()+1 && dr.Rect.Dy() >= 12 {
			found = true
		}
	}
	if !found {
		t.Errorf("active category stripe should tint to current instrument color %v; rects=%v", want, rects)
	}
}

// TestInstMenuAccentIsCurrentInstrument is a direct check on the accent seam.
func TestInstMenuAccentIsCurrentInstrument(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{CurrentInstrument: roster.Snare})
	want := color.RGBAModel.Convert(instColor(roster.Snare)).(color.RGBA)
	got := color.RGBAModel.Convert(comp.menuAccent()).(color.RGBA)
	if got != want {
		t.Errorf("menuAccent() = %v, want instColor(snare) %v", got, want)
	}
}
