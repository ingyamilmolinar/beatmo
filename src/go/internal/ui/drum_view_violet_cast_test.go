//go:build test

package ui

import (
	"image/color"
	"testing"
)

// TestDrumViewBackgroundCarriesVioletCast pins the sequencer background tokens
// (the zebra row stripes and the empty-cell fill) to the same warm
// magenta-violet hue family as the graph pane's `grid-horizon`, so the drum
// view reads as part of the same Vice City "darker→lighter purple/pink"
// surface rather than a cool slate panel. The invariant: red AND blue both
// exceed green (a purple/magenta bias). Cool slate (#181C26: G>R) fails this;
// the violet horizon (#2A1838) passes it.
func TestDrumViewBackgroundCarriesVioletCast(t *testing.T) {
	cases := []struct {
		name string
		c    color.RGBA
	}{
		{"drum-stripe-even", genColorDrumStripeEven},
		{"drum-stripe-odd", genColorDrumStripeOdd},
		{"drum-cell-off", genColorDrumCellOff},
	}
	for _, tc := range cases {
		r, g, b := tc.c.R, tc.c.G, tc.c.B
		if !(r > g && b > g) {
			t.Errorf("%s = #%02X%02X%02X has no violet cast (need R>G and B>G; got R=%d G=%d B=%d)",
				tc.name, r, g, b, r, g, b)
		}
	}
}
