package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestDesktopNodeHighlightReasonableSize ensures that on desktop (simpleDraw=false)
// node highlights do not render oversized rectangles covering a large portion
// of the screen. We approximate by capturing drawRect calls near the node center
// and verifying the outline size is bounded.
func TestDesktopNodeHighlightReasonableSize(t *testing.T) {
	g := New(testLogger)
	g.Layout(800, 600)
	g.simpleDraw = false
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}

	n := g.tryAddNode(5, 5, model.NodeTypeRegular)
	if n == nil {
		t.Fatalf("failed to add node")
	}
	g.nodeRows[n.ID] = 0
	g.nodeAnim[n.ID] = 1

	// Capture drawRect calls and assert any near the node are not huge.
	screen := ebiten.NewImage(g.winW, g.winH)
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	cx := int((x1 + x2) * 0.5)
	cy := int((y1 + y2) * 0.5)
	rpx := int((x2 - x1) * 0.5)
	if rpx < 1 {
		rpx = 1
	}

	var tooBig []image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		// Consider only rects near the node center within a small window
		if r.Dx() > 0 && r.Dy() > 0 {
			mx := (r.Min.X + r.Max.X) / 2
			my := (r.Min.Y + r.Max.Y) / 2
			if abs(mx-cx) < 5 && abs(my-cy) < 5 {
				// Outline should not exceed 4px beyond node sprite radius
				if r.Dx() > 2*rpx+8 || r.Dy() > 2*rpx+8 {
					tooBig = append(tooBig, r)
				}
				// And should never exceed 1/2 of the pane dimensions
				if r.Dx() > g.winW/2 || r.Dy() > g.split.Y/2 {
					tooBig = append(tooBig, r)
				}
			}
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	g.Draw(screen)
	if len(tooBig) > 0 {
		t.Fatalf("oversized node highlight rects: %v", tooBig)
	}
}
