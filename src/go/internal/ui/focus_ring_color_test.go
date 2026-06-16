//go:build test

package ui

import (
	"image/color"
	"testing"
)

// TestFocusRingColor_IsHotPinkSecondAccent pins the focus-ring color to
// hot-pink-300 (#FF2D9E) — the second accent of the Vice City two-accent
// system (cyan primary for interactive/selected/active surfaces, hot-pink
// for focus/selection rings). The ring must stay distinct from the cyan
// `primary` family and from the historical coral (#FFA68C), which was
// visually indistinguishable from the play-pulse halo. DESIGN.md is the
// source of truth; this test pairs with TestDesignMDDrift to keep
// DESIGN.md ↔ runtime aligned.
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
