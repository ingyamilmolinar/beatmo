//go:build test

package ui

import (
	"image/color"
	"testing"
)

// The stripes and empty cells share the graph pane's magenta-violet hue family: red and blue both above green.
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
