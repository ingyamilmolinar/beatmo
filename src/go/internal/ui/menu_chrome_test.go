package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// interceptDrawRect swaps the package drawRect var, forwarding to fn then the
// original, and returns a restore func.
func interceptDrawRect(fn func(*ebiten.Image, image.Rectangle, color.Color, bool)) func() {
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		fn(dst, r, c, filled)
		orig(dst, r, c, filled)
	}
	return func() { drawRect = orig }
}

// TestMenuActiveItemDrawsAzureStripe verifies an active menu item paints a
// left accent stripe in the azure family and a tinted fill; a resting item
// paints neither.
func TestMenuActiveItemDrawsAzureStripe(t *testing.T) {
	dst := ebiten.NewImage(200, 40)
	r := image.Rect(0, 0, 200, 30)

	var stripeSeen bool
	restore := interceptDrawRect(func(_ *ebiten.Image, rect image.Rectangle, _ color.Color, _ bool) {
		if rect.Min.X == r.Min.X && rect.Dx() <= accentStripeW() && rect.Dy() >= r.Dy()-2 {
			stripeSeen = true
		}
	})
	defer restore()

	drawMenuItemBackground(dst, r, menuItemActive)
	if !stripeSeen {
		t.Fatal("active menu item must draw a left azure accent stripe")
	}
}

// TestMenuRestItemDrawsNothing verifies a resting item paints no background.
func TestMenuRestItemDrawsNothing(t *testing.T) {
	dst := ebiten.NewImage(200, 40)
	r := image.Rect(0, 0, 200, 30)
	var drew bool
	restore := interceptDrawRect(func(_ *ebiten.Image, _ image.Rectangle, _ color.Color, _ bool) { drew = true })
	defer restore()
	drawMenuItemBackground(dst, r, menuItemRest)
	if drew {
		t.Fatal("resting menu item must not paint a background")
	}
}

// TestLongPressConnectHoverUsesAzure proves the Connect-hover text is in the
// azure single-accent family, not cyan (viz-only).
func TestLongPressConnectHoverUsesAzure(t *testing.T) {
	got := longPressConnectHoverColor()
	if got != color.Color(colAccent) && got != color.Color(colAccentBright) {
		t.Fatalf("Connect hover must use azure accent, got %v (cyan in chrome is a single-accent violation)", got)
	}
}
