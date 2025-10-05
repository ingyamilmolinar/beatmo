package ui

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestNodeSpriteCacheReusedAcrossFrames(t *testing.T) {
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
	if g.nodeSpriteCache == nil || len(g.nodeSpriteCache) == 0 {
		t.Fatalf("expected node sprite cache entries")
	}
	before := len(g.nodeSpriteCache)
	// Draw again without changes; cache size should remain the same.
	g.drawGridPane(img)
	if len(g.nodeSpriteCache) != before {
		t.Fatalf("unexpected node sprite cache growth: before=%d after=%d", before, len(g.nodeSpriteCache))
	}
	// Pan camera; node sprite cache should still be reused (screen-space size constant per zoom).
	g.cam.OffsetX += 10
	g.cam.OffsetY += 10
	g.cam.Snap()
	g.drawGridPane(img)
	if len(g.nodeSpriteCache) != before {
		t.Fatalf("unexpected node sprite cache growth after pan: before=%d after=%d", before, len(g.nodeSpriteCache))
	}
}
