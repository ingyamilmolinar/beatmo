package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Straight RGB: the rings are the row colour at per-ring alphas, and RGBA() would premultiply.
func colorMatchesRow(c, row color.Color) bool {
	cn := color.NRGBAModel.Convert(c).(color.NRGBA)
	rn := color.NRGBAModel.Convert(row).(color.NRGBA)
	return cn.R == rn.R && cn.G == rn.G && cn.B == rn.B
}

// At least two concentric rings in the node's own colour family, never one hard-edged blob.
func TestDesktopFiringNodeBloomRings(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	assertDefaultSimpleDraw(t, g)
	g.simpleDraw = false
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.pendingStartRow = -1
	g.quietFrames = 0

	n := g.nodeAt(0, 0)
	if n == nil {
		n = g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	if n == nil {
		t.Fatalf("failed to add node")
	}
	g.nodeRows[n.ID] = 0
	g.nodeAnimSet(n.ID, 1)

	rPxOf := func() int {
		x1, _, x2, _ := g.nodeScreenRect(n)
		return int((x2 - x1) * 0.5)
	}
	// The bloom skips tiny nodes, so zoom until rPx ≥ 8.
	for i := 0; i < 6 && rPxOf() < 9; i++ {
		g.cam.Scale *= 1.4
	}

	rowCol := g.drum.Rows[0].Color
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	cx := int((x1 + x2) * 0.5)
	cy := int((y1 + y2) * 0.5)

	var ringCount int
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if filled && !r.Empty() {
			mx := (r.Min.X + r.Max.X) / 2
			my := (r.Min.Y + r.Max.Y) / 2
			if abs(mx-cx) <= 4 && abs(my-cy) <= 4 && colorMatchesRow(c, rowCol) {
				ringCount++
			}
		}
		orig(dst, r, c, radius, filled)
	}
	defer func() { drawRoundedRect = orig }()

	screen := ebiten.NewImage(g.winW, g.winH)
	g.Draw(screen)

	if ringCount < 2 {
		t.Fatalf("expected >=2 bloom rings of the node color near the node, got %d (rPx=%d)", ringCount, rPxOf())
	}
}
