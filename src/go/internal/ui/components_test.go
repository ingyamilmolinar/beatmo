package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestEdgeStyleDrawProgressSingleArrow(t *testing.T) {
	img := ebiten.NewImage(10, 10)
	var cam ebiten.GeoM
	s := EdgeStyle{Color: color.White, Thickness: 1, ArrowSize: 1}

	count := 0
	orig := drawEdgeLine
	drawEdgeLine = func(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM, col color.Color, thick float64) {
		count++
	}
	defer func() { drawEdgeLine = orig }()

	s.DrawProgress(img, 0, 0, 5, 0, &cam, 1)
	if count != 3 {
		t.Fatalf("expected 3 line draws, got %d", count)
	}

	count = 0
	s.DrawProgress(img, 0, 0, 5, 0, &cam, 0.5)
	if count != 1 {
		t.Fatalf("expected 1 line draw for partial edge, got %d", count)
	}
}
