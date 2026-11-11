//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestMenuItemActiveBackgroundUsesAccent verifies the active menu-item stripe
// is drawn in the supplied accent color, so per-instrument menus can tint their
// active row with the instrument's hue instead of the fixed azure.
func TestMenuItemActiveBackgroundUsesAccent(t *testing.T) {
	accent := color.RGBA{190, 120, 60, 255} // an instrument orange
	r := image.Rect(10, 10, 210, 50)
	img := ebiten.NewImage(256, 64)

	rects := collectFilledRects(t, func() {
		drawMenuItemBackgroundAccent(img, r, menuItemActive, accent)
	})

	// The solid left stripe must be drawn in the accent color.
	found := false
	for _, dr := range rects {
		if dr.Color == accent && dr.Rect.Min.X == r.Min.X && dr.Rect.Dx() <= accentStripeW()+1 {
			found = true
		}
	}
	if !found {
		t.Errorf("active menu item should draw a solid accent stripe in %v; rects=%v", accent, rects)
	}
}

// TestMenuHeaderBandAccentUsesAccent verifies the header underline is tinted by
// the supplied accent color.
func TestMenuHeaderBandAccentUsesAccent(t *testing.T) {
	accent := color.RGBA{60, 150, 170, 255} // an instrument teal
	r := image.Rect(0, 0, 200, 40)
	img := ebiten.NewImage(256, 64)

	rects := collectFilledRects(t, func() {
		drawMenuHeaderBandAccent(img, r, accent)
	})

	// The 1px underline carries the accent hue (alpha-blended). Assert a thin
	// bottom rect exists whose RGB derives from the accent (R/G/B nonzero match
	// direction): teal has G,B > R.
	found := false
	for _, dr := range rects {
		if dr.Rect.Dy() == 1 && dr.Rect.Max.Y == r.Max.Y {
			if dr.Color.G >= dr.Color.R && dr.Color.B >= dr.Color.R && (dr.Color.G > 0 || dr.Color.B > 0) {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("header underline should be tinted by accent %v; rects=%v", accent, rects)
	}
}

// TestMenuChromeDefaultsUnchanged verifies the original (non-accent) helpers
// still draw with colAccent — global menus must be unaffected.
func TestMenuChromeDefaultsUnchanged(t *testing.T) {
	r := image.Rect(10, 10, 210, 50)
	img := ebiten.NewImage(256, 64)

	rects := collectFilledRects(t, func() {
		drawMenuItemBackground(img, r, menuItemActive)
	})
	want := color.RGBAModel.Convert(colAccent).(color.RGBA)
	found := false
	for _, dr := range rects {
		if dr.Color == want && dr.Rect.Min.X == r.Min.X && dr.Rect.Dx() <= accentStripeW()+1 {
			found = true
		}
	}
	if !found {
		t.Errorf("default menu item stripe must still be colAccent %v; rects=%v", want, rects)
	}
}
