package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestDesktopNodeGlowReasonableSize ensures glow radius is clamped to a
// reasonable fraction of the pane height on desktop.
func TestDesktopNodeGlowReasonableSize(t *testing.T) {
	g := New(testLogger)
	g.Layout(800, 600)
	g.simpleDraw = false
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatalf("failed to add node")
	}
	g.nodeRows[n.ID] = 0
	g.nodeAnim[n.ID] = 1
	screen := ebiten.NewImage(g.winW, g.winH)
	g.Draw(screen)
	if g.lastGlowScr <= 0 {
		t.Fatalf("expected glow to be drawn, got lastGlowScr=%.2f", g.lastGlowScr)
	}
	if g.lastGlowScr > float64(g.split.Y)/8+1 {
		t.Fatalf("glow too large: got %.2f want <= %.2f", g.lastGlowScr, float64(g.split.Y)/8)
	}
}
