//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestInstMenuCategoryActiveTintsToCurrentInstrument verifies the instrument
// picker's chrome (here, the active CATEGORY row stripe) tints to the picker's
// owning instrument color (props.CurrentInstrument), not the fixed azure.
func TestInstMenuCategoryActiveTintsToCurrentInstrument(t *testing.T) {
	assertDefaultParityState(t)

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 200, 124),
		VertBounds:        image.Rect(0, 50, 400, 700),
		CurrentInstrument: "kick",
		Categories:        []string{"Kicks", "Snares"},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: "Kicks"},
			{ID: "snare", Label: "Snare", Category: "Snares"},
		},
		RowHeight:       28,
		LabelWidth:      120,
		ControlsWidth:   200,
		ForceCategories: true,
	})
	comp.Open()
	comp.state.activeCat = "Kicks"
	comp.state.mode = InstMenuModeCategories
	comp.rebuildMenu()

	want := color.RGBAModel.Convert(instColor("kick")).(color.RGBA)
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
	comp.SetProps(InstrumentMenuProps{CurrentInstrument: "snare"})
	want := color.RGBAModel.Convert(instColor("snare")).(color.RGBA)
	got := color.RGBAModel.Convert(comp.menuAccent()).(color.RGBA)
	if got != want {
		t.Errorf("menuAccent() = %v, want instColor(snare) %v", got, want)
	}
}
