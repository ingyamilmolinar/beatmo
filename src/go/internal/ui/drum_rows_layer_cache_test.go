package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestRowsLayerCacheRebuild verifies the rows composite layer is reused across
// unchanged frames and rebuilt when offset or row content changes.
func TestRowsLayerCacheRebuild(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)

	// Minimal setup: one row, small length
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.drum.SetBeatLength(16)
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.Rows[0].Steps[0] = true
	g.drum.Bounds = image.Rect(0, 300, 800, 600)
	g.drum.calcLayout()

	dst := ebiten.NewImage(800, 600)
	// First draw builds caches
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	gen1 := g.drum.rowsLayerGen
	if gen1 == 0 {
		t.Fatalf("expected rowsLayer to be built")
	}
	// Second draw with no changes should reuse
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	if g.drum.rowsLayerGen != gen1 {
		t.Fatalf("rowsLayer should be reused; got gen %d -> %d", gen1, g.drum.rowsLayerGen)
	}
	// Change offset -> should rebuild layer (but reuse row sprites via shift)
	g.drum.Offset++
	g.drum.markRowsShiftDirty()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	gen2 := g.drum.rowsLayerGen
	if gen2 == gen1 {
		t.Fatalf("expected rowsLayer rebuild on offset shift")
	}
	// Change row content -> should rebuild row sprite and layer
	g.drum.Rows[0].Steps[1] = true
	g.drum.markRowDirty(0)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	if g.drum.rowsLayerGen == gen2 {
		t.Fatalf("expected rowsLayer rebuild after row content change")
	}
}
