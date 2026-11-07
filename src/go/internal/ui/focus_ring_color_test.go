//go:build test

package ui

import (
	"image/color"
	"testing"
)

// Hot-pink, the focus accent, distinct from the primary family and from the coral that read as the play-pulse halo.
func TestFocusRingColor_IsHotPinkSecondAccent(t *testing.T) {
	want := color.RGBA{R: 255, G: 45, B: 158, A: 255} // #FF2D9E
	if genColorFocusRing != want {
		t.Fatalf("genColorFocusRing = %#v, want %#v (DESIGN.md focus-ring should be #FF2D9E)", genColorFocusRing, want)
	}
	// And the token must NOT be the historical coral.
	coral := color.RGBA{R: 255, G: 166, B: 140, A: 255}
	if genColorFocusRing == coral {
		t.Fatalf("genColorFocusRing reverted to #FFA68C coral — confuses with play-pulse halo")
	}
	// Nor the cyan primary — focus must read as its own accent.
	if genColorFocusRing == genColorPrimary {
		t.Fatalf("genColorFocusRing must differ from primary (two-accent system)")
	}
}
