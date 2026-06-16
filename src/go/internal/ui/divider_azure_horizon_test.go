//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDividerIsAzureHorizon pins the split divider to the azure chrome accent
// (the Vice City "horizon line"), not the old bright-gray hairline that
// out-shone every other element. Reads the divider row back from a real image
// and asserts an azure hue (blue-dominant: B>G>R, matching #4FB4FF), which
// holds for both the rest (colAccent) and hover (colAccentBright) variants.
func TestDividerIsAzureHorizon(t *testing.T) {
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720) // desktop width → non-mobile divider path

	if !g.split.horizontal {
		g.split.horizontal = true
		g.split.Y = 360
	}
	img := ebiten.NewImage(1280, 720)
	g.drawDivider(img)

	// Sample the divider row away from the centre handle.
	r, gr, b, a := img.At(40, g.split.Y).RGBA()
	r8, g8, b8, a8 := uint8(r>>8), uint8(gr>>8), uint8(b>>8), uint8(a>>8)
	if a8 == 0 {
		t.Fatalf("divider row (40,%d) is transparent — nothing drawn", g.split.Y)
	}
	if !(b8 > g8 && g8 > r8) {
		t.Errorf("divider colour #%02X%02X%02X is not azure (want blue-dominant B>G>R like the accent)", r8, g8, b8)
	}
}
