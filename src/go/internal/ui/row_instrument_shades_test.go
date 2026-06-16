//go:build test

package ui

import (
	"image/color"
	"testing"
)

// lum is a quick perceptual-ish brightness proxy for shade comparisons.
func lum(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }

// TestRowToggleShadesDeriveFromInstrumentColor verifies that the per-row
// toggle fills are shades of the instrument color: mute darker, solo brighter,
// fx the full hue, and the off state a dim (low-alpha) tint — all distinct.
func TestRowToggleShadesDeriveFromInstrumentColor(t *testing.T) {
	base := color.RGBA{180, 120, 60, 255} // a mid-tone orange instrument color

	mute := rowToggleFillRGBA(base, roleMute, true)
	solo := rowToggleFillRGBA(base, roleSolo, true)
	fx := rowToggleFillRGBA(base, roleFX, true)
	off := rowToggleFillRGBA(base, roleMute, false)

	baseLum := lum(base)

	if lum(mute) >= baseLum {
		t.Errorf("mute active fill should be DARKER than base: mute lum %d, base lum %d", lum(mute), baseLum)
	}
	if lum(solo) <= baseLum {
		t.Errorf("solo active fill should be BRIGHTER than base: solo lum %d, base lum %d", lum(solo), baseLum)
	}
	if fx.R != base.R || fx.G != base.G || fx.B != base.B {
		t.Errorf("fx active fill should be the full instrument hue: got %v, want %v", fx, base)
	}
	if off.A >= 200 {
		t.Errorf("off fill should be a dim (low-alpha) tint, got alpha %d", off.A)
	}

	// The three active states must all be visually distinct from each other
	// and from the dim off state.
	if mute == solo || mute == fx || solo == fx {
		t.Errorf("mute/solo/fx active fills must all differ: mute=%v solo=%v fx=%v", mute, solo, fx)
	}
}

// TestRowToggleStyleDiffersByInstrumentColor verifies two differently-colored
// rows produce different toggle fills — the whole point of tying color to the
// instrument (no more shared light-blue drift).
func TestRowToggleStyleDiffersByInstrumentColor(t *testing.T) {
	orange := color.RGBA{180, 120, 60, 255}
	teal := color.RGBA{60, 150, 160, 255}

	mOrange := rowToggleStyle(orange, roleMute, true)
	mTeal := rowToggleStyle(teal, roleMute, true)

	if colorsEqual(mOrange.Fill, mTeal.Fill) {
		t.Errorf("differently-colored rows must yield different mute fills: %v vs %v", mOrange.Fill, mTeal.Fill)
	}
	if mOrange.Radius != RadiusSM {
		t.Errorf("row toggle radius should be RadiusSM (%d), got %d", RadiusSM, mOrange.Radius)
	}
}

// TestRowToggleNilColorIsSafe guards the test/headless paths that build
// DrumRow values without a color.
func TestRowToggleNilColorIsSafe(t *testing.T) {
	// Must not panic and must return a usable style.
	st := rowToggleStyle(nil, roleSolo, true)
	if st.Fill == nil {
		t.Error("nil instrument color should fall back to a non-nil fill")
	}
}
