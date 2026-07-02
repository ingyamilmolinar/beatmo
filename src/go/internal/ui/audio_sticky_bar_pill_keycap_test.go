package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestPillKeycapEngageAndTravel(t *testing.T) {
	// drawPillTabAt renders the keycap itself and ignores Button.Style, so the
	// style passed here is irrelevant to the behavior under test.
	b := NewButton("EQ", InstButtonStyle, nil)
	b.SetRect(image.Rect(0, 0, 40, 22))
	dst := ebiten.NewImage(40, 22)

	drawPillTabAt(dst, b, false)
	if b.engageAnim != 0 {
		t.Fatalf("inactive pill engageAnim = %v, want 0", b.engageAnim)
	}
	drawPillTabAt(dst, b, true)
	if b.engageAnim <= 0 {
		t.Fatalf("off→on did not fire engage flash (got %v)", b.engageAnim)
	}
	prev := b.engageAnim
	drawPillTabAt(dst, b, true)
	if b.engageAnim >= prev {
		t.Fatalf("engage flash re-fired/stuck while staying active (%v → %v)", prev, b.engageAnim)
	}
	for i := 0; i < 80; i++ {
		drawPillTabAt(dst, b, true)
	}
	if b.engageAnim != 0 {
		t.Fatalf("engage flash did not settle to 0 (got %v)", b.engageAnim)
	}

	b.HandleInputResult(20, 11, true) // press at center
	for i := 0; i < 12; i++ {
		drawPillTabAt(dst, b, true)
	}
	if px := b.pressTravelPx(); px < genGeomKeycapWallDepth-1 {
		t.Fatalf("held pill travel = %dpx, want ~%d", px, genGeomKeycapWallDepth)
	}
}
