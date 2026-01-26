package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestSimpleDrawNodeHighlightThickOutline ensures that in simple draw mode,
// node highlights render with a thick, high-contrast outline (multiple
// drawRect border calls, including white and row-colored rings).
func TestSimpleDrawNodeHighlightThickOutline(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 360)
	assertDefaultSimpleDraw(t, g)
	g.simpleDraw = true
	g.pendingStartRow = -1
	// Ensure row 0 exists and has a known color.
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	rowColor := color.RGBA{8, 160, 240, 255}
	g.drum.SetRowColor(0, rowColor)

	// Add a regular node and mark it animating.
	n := g.nodeAt(0, 0)
	if n == nil {
		n = g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	if n == nil {
		t.Fatalf("failed to add node")
	}
	g.nodeRows[n.ID] = 0   // map node to row 0 for color
	g.nodeAnimSet(n.ID, 1) // active highlight
	g.quietFrames = 0

	// Intercept drawRect to capture border strokes.
	var borderCount int
	var whiteCount int
	var rowColCount int
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if !filled {
			borderCount++
			rgba := color.RGBAModel.Convert(c).(color.RGBA)
			if rgba == (color.RGBA{255, 255, 255, 255}) {
				whiteCount++
			}
			if rgba == rowColor {
				rowColCount++
			}
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	screen := ebiten.NewImage(g.winW, g.winH)
	g.Draw(screen)

	if borderCount < 3 {
		t.Fatalf("expected >=3 border strokes for thick outline, got %d", borderCount)
	}
	if whiteCount < 2 {
		t.Fatalf("expected >=2 white outline strokes, got %d", whiteCount)
	}
	if rowColCount < 1 {
		t.Fatalf("expected 1 inner row-colored outline stroke, got %d", rowColCount)
	}
}
