//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRowAccentStripeWidthMatchesProfile asserts the profile exposes a
// non-zero accent-stripe width on mobile, which the row draw path uses as
// the column of instrument-colored pixels at rowRect.Min.X. The stripe gives
// each row a distinct visual identity — it's how the eye groups
// "this row plays kick" without reading the row name.
func TestRowAccentStripeWidthMatchesProfile(t *testing.T) {
	assertDefaultParityState(t)

	desktopWidth := genDesktopProfile.AccentStripeWidth
	mobileWidth := genMobileProfile.AccentStripeWidth
	if desktopWidth <= 0 {
		t.Errorf("desktop AccentStripeWidth should be >0 to render the stripe, got %d", desktopWidth)
	}
	if mobileWidth <= 0 {
		t.Errorf("mobile AccentStripeWidth should be >0 to render the stripe, got %d", mobileWidth)
	}
	if mobileWidth < desktopWidth {
		t.Errorf("mobile AccentStripeWidth (%d) should not be smaller than desktop (%d) — touch UIs need the stronger anchor",
			mobileWidth, desktopWidth)
	}
}

// TestRowColorStripeAcceptsAnyColor exercises drawAccentStripe with varied
// instrument colors to confirm the helper does not crash and respects the
// caller's color choice. (Pixel-perfect verification of the rendered RGBA
// lives in the regenerated visual baseline.)
func TestRowColorStripeAcceptsAnyColor(t *testing.T) {
	assertDefaultParityState(t)

	colors := []color.Color{
		color.RGBA{255, 80, 60, 255},
		color.RGBA{60, 220, 140, 255},
		color.RGBA{120, 130, 255, 255},
	}
	rect := image.Rect(0, 0, 3, TouchRowHeight())
	canvas := ebiten.NewImage(8, TouchRowHeight()+4)
	for _, c := range colors {
		// Direct call — drawAccentStripe is the helper used at row draw time;
		// the test confirms it does not panic for typical instrument colors.
		drawAccentStripe(canvas, rect, c)
	}
}
