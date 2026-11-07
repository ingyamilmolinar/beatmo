//go:build test

package ui

import (
	"image/color"
	"testing"
)

// lum is a quick perceptual-ish brightness proxy for shade comparisons.
func lum(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }

// Mute darker, solo brighter, fx the full hue, all distinct; off is the neutral body.
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
	if want := rowShadeToRGBA(InstButtonStyle.fill()); off != want {
		t.Errorf("off fill should be the neutral row-control body %v, got %v", want, off)
	}

	if mute == solo || mute == fx || solo == fx {
		t.Errorf("mute/solo/fx active fills must all differ: mute=%v solo=%v fx=%v", mute, solo, fx)
	}
}

// The point of tying colour to the instrument.
func TestRowToggleStyleDiffersByInstrumentColor(t *testing.T) {
	orange := color.RGBA{180, 120, 60, 255}
	teal := color.RGBA{60, 150, 160, 255}

	mOrange := rowToggleStyle(orange, roleMute, true)
	mTeal := rowToggleStyle(teal, roleMute, true)

	if colorsEqual(mOrange.Fill, mTeal.Fill) {
		t.Errorf("differently-colored rows must yield different mute fills: %v vs %v", mOrange.Fill, mTeal.Fill)
	}
	if mOrange.Radius != RadiusControl {
		t.Errorf("row toggle radius should be RadiusControl (%d), got %d", RadiusControl, mOrange.Radius)
	}
}

// Headless paths build DrumRow values with no colour.
func TestRowToggleNilColorIsSafe(t *testing.T) {
	// Must not panic and must return a usable style.
	st := rowToggleStyle(nil, roleSolo, true)
	if st.Fill == nil {
		t.Error("nil instrument color should fall back to a non-nil fill")
	}
}

// WCAG 4.5:1 on every fill: a pale instrument gets a dark letter, a deep one a light letter.
func TestRowToggleLetterIsLegibleOnEveryInstrumentColor(t *testing.T) {
	for _, base := range genInstrumentSequence {
		for _, role := range []rowToggleRole{roleMute, roleSolo, roleFX} {
			fill := rowToggleFillRGBA(base, role, true)
			txt := rowToggleLabelColor(base, role, true)
			if r := wcagContrast(relLuminance(fill), relLuminance(txt)); r < 4.5 {
				t.Errorf("instrument %v role %d: letter %v on %v contrast %.2f < 4.5", base, role, txt, fill, r)
			}
		}
	}
}
