package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestDrawCullingSkipsOffscreen verifies edges and nodes inside the viewport
// are drawn, then culling skips them after a large pan.
func TestDrawCullingSkipsOffscreen(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Place a small rectangle at the origin (initially visible).
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	img := ebiten.NewImage(800, g.split.Y)
	g.drawGridPane(img)
	if g.lastDrawNodes == 0 || g.lastDrawEdges == 0 {
		t.Fatalf("expected initial elements to be drawn, got nodes=%d edges=%d", g.lastDrawNodes, g.lastDrawEdges)
	}
	// Pan camera far away so the origin moves offscreen and draw again.
	g.cam.OffsetX = 10000
	g.cam.OffsetY = 10000
	g.cam.Snap()
	g.drawGridPane(img)
	if g.lastDrawNodes != 0 || g.lastDrawEdges != 0 {
		t.Fatalf("expected elements to be culled after large pan, drew nodes=%d edges=%d", g.lastDrawNodes, g.lastDrawEdges)
	}
}
