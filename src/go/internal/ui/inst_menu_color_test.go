//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestInstrumentMenuActiveRowTintsToInstrumentColor verifies the currently-
// selected instrument row's active stripe is rendered in that instrument's own
// color, so the picker's highlight matches its swatch (colors informational and
// consistent) instead of the fixed azure accent.
func TestInstrumentMenuActiveRowTintsToInstrumentColor(t *testing.T) {
	assertDefaultParityState(t)

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 200, 124),
		VertBounds:        image.Rect(0, 50, 400, 700),
		CurrentInstrument: "kick",
		Categories:        []string{"Kicks"},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: "Kicks"},
			{ID: "snare", Label: "Snare", Category: "Kicks"},
		},
		RowHeight:     28,
		LabelWidth:    120,
		ControlsWidth: 200,
	})
	comp.Open()
	comp.state.mode = InstMenuModeInstruments
	comp.rebuildMenu()

	want := color.RGBAModel.Convert(instColor("kick")).(color.RGBA)
	azure := color.RGBAModel.Convert(colAccent).(color.RGBA)
	if want == azure {
		t.Skip("kick color coincides with azure; pick a different fixture")
	}

	img := ebiten.NewImage(400, 700)
	rects := collectFilledRects(t, func() { comp.Draw(img) })

	// The active stripe is a thin, full-row-height bar at a row's left edge.
	// Require Dy tall enough to exclude the swatch's rounded-corner slivers.
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
