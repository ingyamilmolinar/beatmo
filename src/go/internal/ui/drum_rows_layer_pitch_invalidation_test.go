package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// SetLength invalidates the row caches and marks every row dirty, so the shift path cannot keep stale-pitch pixels,
// and rowsLayerLength tracks dv.Length for the reuse predicates.
func TestRowsLayerInvalidatesOnLengthChange(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1600, 600)

	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.drum.SetLength(16)
	g.drum.SetBounds(image.Rect(0, 300, 1600, 600))

	dst := ebiten.NewImage(1600, 600)
	g.drum.Draw(dst, nil, 0, nil, 0)
	gen0 := g.drum.rowsLayerGen
	if gen0 == 0 {
		t.Fatalf("expected rowsLayer to be built on first draw")
	}
	if g.drum.rowsLayerLength != g.drum.Length {
		t.Fatalf("rowsLayerLength=%d should track dv.Length=%d after build", g.drum.rowsLayerLength, g.drum.Length)
	}
	if g.drum.rowsLayerRowWidth != g.drum.cellGridRect.Dx() {
		t.Fatalf("rowsLayerRowWidth=%d should track cellGridRect.Dx()=%d after build", g.drum.rowsLayerRowWidth, g.drum.cellGridRect.Dx())
	}

	// Sanity: an unchanged second draw reuses the layer.
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowsLayerGen != gen0 {
		t.Fatalf("expected rowsLayer reuse when nothing changed; gen %d -> %d", gen0, g.drum.rowsLayerGen)
	}

	// SetLength must invalidate. 128 vs 16 = 8x finer pitch.
	g.drum.SetLength(128)
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowsLayerGen == gen0 {
		t.Fatalf("expected rowsLayer rebuild after SetLength; gen unchanged at %d", gen0)
	}
	if g.drum.rowsLayerLength != 128 {
		t.Fatalf("rowsLayerLength=%d should equal new dv.Length=128", g.drum.rowsLayerLength)
	}
}

// A narrower timeline forces a rebuild even with Bounds and baseX unchanged: reuse must check rowWidth.
func TestRowsLayerInvalidatesOnTimelineWidthChange(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1600, 600)

	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.drum.SetLength(32)
	g.drum.SetBounds(image.Rect(0, 300, 1600, 600))

	dst := ebiten.NewImage(1600, 600)
	g.drum.Draw(dst, nil, 0, nil, 0)
	gen0 := g.drum.rowsLayerGen
	if gen0 == 0 {
		t.Fatalf("expected rowsLayer to be built on first draw")
	}
	originalWidth := g.drum.cellGridRect.Dx()
	if originalWidth <= 200 {
		t.Fatalf("test requires cellGridRect.Dx() > 200, got %d", originalWidth)
	}

	// The right edge shrinks while baseX stays put.
	g.drum.cellGridRect.Max.X -= 200
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowsLayerGen == gen0 {
		t.Fatalf("expected rowsLayer rebuild after cellGridRect.Dx() shrank from %d to %d; gen unchanged at %d",
			originalWidth, g.drum.cellGridRect.Dx(), gen0)
	}
	if g.drum.rowsLayerRowWidth != g.drum.cellGridRect.Dx() {
		t.Fatalf("rowsLayerRowWidth=%d should equal new cellGridRect.Dx()=%d after rebuild",
			g.drum.rowsLayerRowWidth, g.drum.cellGridRect.Dx())
	}
}
