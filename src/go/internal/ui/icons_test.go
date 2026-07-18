package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSeekIconsDraw(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	for _, id := range []IconID{IconRewind, IconFastForward} {
		if !DrawIcon(dst, id, r, colTextPrimary) {
			t.Fatalf("DrawIcon returned false for %q (not registered in switch)", id)
		}
	}
}
