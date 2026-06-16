package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestEdgeCacheRebuildOnGraphChange(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
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
	// Add another edge -> should rebuild (re-render the edges into the cache).
	// The rebuild REUSES the same texture (Clear()+redraw) rather than allocating
	// a fresh image: per-frame edge-cache reallocation is the atlas churn that
	// stalls the single WASM thread and starves audio during a zoom, so the fix in
	// grid_pane_draw.go reuses the image when its dimensions are unchanged. Rebuild
	// is therefore detected by the edge count increasing, not by image identity —
	// and we additionally pin the no-realloc guarantee (same image object).
	n2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.drawGridPane(img)
	if g.edgeCacheCount <= cnt {
		t.Fatalf("expected edge cache rebuild (more edges drawn): before=%d after=%d", cnt, g.edgeCacheCount)
	}
	if g.edgeCache != first {
		t.Fatalf("edge cache reallocated on rebuild; it must reuse the same texture " +
			"(Clear()+redraw) when dimensions are unchanged — per-frame realloc is the " +
			"atlas churn that starves audio during a zoom")
	}
}
