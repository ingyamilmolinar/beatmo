package ui

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestEdgeCacheRebuildOnGraphChange(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	img := ebiten.NewImage(800, g.split.Y)
	g.drawGridPane(img)
	if g.edgeCache == nil || g.edgeCacheCount == 0 {
		t.Fatalf("expected edge cache to be built (count=%d)", g.edgeCacheCount)
	}
	first := g.edgeCache
	cnt := g.edgeCacheCount
	// Draw again with no changes -> same cache image
	g.drawGridPane(img)
	if g.edgeCache != first || g.edgeCacheCount != cnt {
		t.Fatalf("unexpected edge cache rebuild without changes")
	}
	// Add another edge -> should rebuild
	n2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.drawGridPane(img)
	if g.edgeCache == first || g.edgeCacheCount <= cnt {
		t.Fatalf("expected edge cache rebuild and increased count: before=%d after=%d", cnt, g.edgeCacheCount)
	}
}
