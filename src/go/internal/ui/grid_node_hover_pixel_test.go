//go:build test

package ui

import (
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// hoverMaxChannelDelta draws the grid twice (with and without the node hovered)
// and returns the largest per-channel pixel difference anywhere in a box around
// the node. This is a real VISUAL measurement (reads composited pixels via the
// ebiten stub — real Ebiten panics on At() outside RunGame, which is why every
// pixel test here is `//go:build test`). Draw behaviour is identical stub↔real
// (verified via the drawRoundedRect interceptor), so the stub faithfully
// measures what the user sees.
func hoverMaxChannelDelta(t *testing.T) int {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.simpleDraw = false
	g.cam.OffsetX = float64(g.winW) / 2
	g.cam.OffsetY = float64(g.split.Y-gridTopOffset()) / 2

	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	n := g.nodeAt(0, 0)
	if n == nil {
		n = g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	if n == nil {
		t.Fatal("failed to add node")
	}
	g.nodeRows[n.ID] = 0
	rPxOf := func() int { x1, _, x2, _ := g.nodeScreenRect(n); return int((x2 - x1) * 0.5) }
	for i := 0; i < 8 && rPxOf() < 12; i++ {
		g.cam.Scale *= 1.3
	}
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	cx := int(math.Round((x1 + x2) / 2))
	cy := int(math.Round((y1 + y2) / 2))
	if cy >= g.split.Y || cy < gridTopOffset() {
		t.Skipf("node outside grid pane at (%d,%d)", cx, cy)
	}
	rPx := rPxOf()

	read := func() map[[2]int]color.RGBA {
		screen := ebiten.NewImage(640, 480)
		g.Draw(screen)
		m := map[[2]int]color.RGBA{}
		for dy := -(rPx + 10); dy <= rPx+10; dy++ {
			for dx := -(rPx + 10); dx <= rPx+10; dx++ {
				p := [2]int{cx + dx, cy + dy}
				m[p] = color.RGBAModel.Convert(screen.At(p[0], p[1])).(color.RGBA)
			}
		}
		return m
	}

	g.hover = nil
	for i := 0; i < 5; i++ {
		g.advanceNodeHoverGlow()
	}
	base := read()

	g.hover = n
	for i := 0; i < 40; i++ {
		g.advanceNodeHoverGlow()
	}
	hover := read()

	md := 0
	for p, ca := range base {
		cb := hover[p]
		for _, d := range []int{iabsHover(int(ca.R) - int(cb.R)), iabsHover(int(ca.G) - int(cb.G)), iabsHover(int(ca.B) - int(cb.B))} {
			if d > md {
				md = d
			}
		}
	}
	return md
}

func iabsHover(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// hoverVisibleMinDelta is the minimum per-channel pixel change a hover must
// produce to count as a clearly-visible highlight. The pre-fix faint bloom
// (behind the node, ~alpha 9–63) measured only ~59 and was invisible in
// practice; a clear hover affordance lands well above this.
const hoverVisibleMinDelta = 110

// TestGridNodeHoverIsVisiblyHighlighted is the TRUE visual guard the earlier
// interceptor tests lacked: it asserts hovering a node produces a clearly
// perceptible on-screen change, not merely that drawNodeBloom was called.
func TestGridNodeHoverIsVisiblyHighlighted(t *testing.T) {
	md := hoverMaxChannelDelta(t)
	t.Logf("hover max channel delta = %d (min visible = %d)", md, hoverVisibleMinDelta)
	if md < hoverVisibleMinDelta {
		t.Fatalf("hovering a node changed pixels by only %d/255 — the highlight is not visibly perceptible (want >= %d)", md, hoverVisibleMinDelta)
	}
}
