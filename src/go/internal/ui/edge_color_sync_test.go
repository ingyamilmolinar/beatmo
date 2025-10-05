//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestEdgeColorTracksRowColor ensures edges are drawn with the same color as
// their row's node color and that changing the row color rebuilds the cache.
func TestEdgeColorTracksRowColor(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Build a simple edge on row 0 from (0,0) -> (1,0)
	n0 := g.tryAddNode(0, 0, 0)
	n1 := g.tryAddNode(1, 0, 0)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	// Override drawEdgeLine to capture last color used for edges
	var lastCol color.Color
	prev := drawEdgeLine
	drawEdgeLine = func(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM, col color.Color, thick float64) {
		lastCol = col
	}
	defer func() { drawEdgeLine = prev }()

	img := ebiten.NewImage(640, g.split.Y)

	// Set initial row color to red and draw
	red := color.RGBA{255, 0, 0, 255}
	g.drum.SetRowColor(0, red)
	g.drawGridPane(img)
	if lastCol == nil {
		t.Fatalf("edge not drawn")
	}
	if r, _, _, _ := lastCol.RGBA(); r>>8 != 255 {
		t.Fatalf("expected edge color red, got %#v", lastCol)
	}

	// Change row color to green and draw; expect cache rebuild and new color
	lastCol = nil
	green := color.RGBA{0, 255, 0, 255}
	g.drum.SetRowColor(0, green)
	g.drawGridPane(img)
	if lastCol == nil {
		t.Fatalf("edge not redrawn after color change")
	}
	if r, gc, _, _ := lastCol.RGBA(); (r>>8) != 0 || (gc>>8) != 255 {
		t.Fatalf("expected edge color green, got %#v", lastCol)
	}
}
