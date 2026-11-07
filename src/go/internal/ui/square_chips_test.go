//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A matte keycap with the shared secondary fill, like every other Button.
func TestBeatCounterRendersRoundedKeycap(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 28)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	if _, ok := beatCounterKeycapCap(t, z, dst, beatRect); !ok {
		t.Fatalf("beat counter did not render a keycap cap face inside %v", beatRect)
	}
}
