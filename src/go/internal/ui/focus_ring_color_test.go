//go:build test

package ui

import (
	"image/color"
	"testing"
)

// TestFocusRingColor_IsNeutralWhiteNotCoral pins the focus-ring color to
// neutral high-contrast white. Regression for A4: the previous coral
// (#FFA68C) was visually indistinguishable from the play-pulse halo
// (#FF8C5A) when both were on screen, so we deliberately switched the
// focus-ring token to white. DESIGN.md is the source of truth; this test
// pairs with TestDesignMDDrift to keep DESIGN.md ↔ runtime aligned.
func TestFocusRingColor_IsNeutralWhiteNotCoral(t *testing.T) {
	want := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if genColorFocusRing != want {
		t.Fatalf("genColorFocusRing = %#v, want %#v (DESIGN.md focus-ring should be #FFFFFF)", genColorFocusRing, want)
	}
	// And the token must NOT be the historical coral.
	coral := color.RGBA{R: 255, G: 166, B: 140, A: 255}
	if genColorFocusRing == coral {
		t.Fatalf("genColorFocusRing reverted to #FFA68C coral — confuses with play-pulse halo")
	}
}
