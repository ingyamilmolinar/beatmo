package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestKnurledCapSpriteIsCachedByDiameter(t *testing.T) {
	knurledCapCache = map[int]*ebiten.Image{} // reset
	a := knurledCapSprite(48)
	b := knurledCapSprite(48)
	if a != b {
		t.Fatalf("same diameter must return the same cached sprite")
	}
	c := knurledCapSprite(40)
	if c == a {
		t.Fatalf("different diameter must produce a different sprite")
	}
	if got := len(knurledCapCache); got != 2 {
		t.Fatalf("cache size = %d, want 2 (48,40)", got)
	}
}
