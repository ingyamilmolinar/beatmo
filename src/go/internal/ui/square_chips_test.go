//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestBeatCounterRendersRoundedKeycap verifies the beat/time counter renders as
// a matte keycap (a rounded cap face with the shared secondary fill), matching
// every other Button per DESIGN.md "Keycap elevation" — the old flat square
// chip did not comply. Squareness is no longer asserted; the keycap is rounded
// by design.
func TestBeatCounterRendersRoundedKeycap(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 28)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	if _, ok := beatCounterKeycapCap(t, z, dst, beatRect); !ok {
		t.Fatalf("beat counter did not render a keycap cap face inside %v", beatRect)
	}
}
