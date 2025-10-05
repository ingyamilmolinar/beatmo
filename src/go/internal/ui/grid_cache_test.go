package ui

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestGridTileCacheRebuildOnScaleChange verifies the grid tile is cached
// across pans but rebuilt when scale changes.
func TestGridTileCacheRebuildOnScaleChange(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)
	// First draw builds the tile.
	img := ebiten.NewImage(800, g.split.Y)
	g.drawGridPane(img)
	if g.gridTile == nil {
		t.Fatalf("gridTile not initialized")
	}
	first := g.gridTile
	step := g.gridTileStepPx
	// Pan camera by a few pixels; tile should be reused.
	g.cam.OffsetX += 5
	g.cam.Snap()
	g.drawGridPane(img)
	if g.gridTile != first {
		t.Fatalf("gridTile should be reused on pan")
	}
	if g.gridTileStepPx != step {
		t.Fatalf("stepPx changed unexpectedly")
	}
	// Change scale slightly; tile should rebuild.
	g.cam.Scale *= 1.1
	g.cam.Snap()
	g.drawGridPane(img)
	if g.gridTile == first {
		t.Fatalf("gridTile should rebuild on scale change")
	}
}
