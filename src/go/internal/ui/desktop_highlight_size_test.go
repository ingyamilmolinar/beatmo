package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The rects drawn near the node stay bounded, never covering a large part of the screen.
func TestDesktopNodeHighlightReasonableSize(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	assertDefaultSimpleDraw(t, g)
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
				// The ring plus a soft glow band adds a few px each side; a runaway highlight is still caught by the half-pane
				// guard below.
				if r.Dx() > 2*rpx+16 || r.Dy() > 2*rpx+16 {
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
