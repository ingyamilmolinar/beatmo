package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRowsLayerInvalidatesOnLengthChange verifies that mutating dv.Length via
// SetLength forces a rowsLayer full rebuild and updates rowsLayerLength.
//
// Before the fix in drumview_transport.go, SetLength only set bgDirty=true
// and relied on the per-row sprite cache's needsRowRebuild() safety net to
// trigger an indirect full rebuild. After the fix, SetLength explicitly
// invalidates row caches and marks all rows dirty so the rowsLayer cannot
// retain stale-pitch pixels via its shift-and-fill path.
//
// This test also verifies the new rowsLayerLength field tracks dv.Length
// after every rebuild — the contract that drumview_cache_rows_layer.go's
// canReuse/needFull predicates depend on.
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
	if g.drum.rowsLayerRowWidth != g.drum.timelineRect.Dx() {
		t.Fatalf("rowsLayerRowWidth=%d should track timelineRect.Dx()=%d after build", g.drum.rowsLayerRowWidth, g.drum.timelineRect.Dx())
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

// TestRowsLayerInvalidatesOnTimelineWidthChange verifies that mutating
// timelineRect.Dx() (e.g. when right-side controls expand and squeeze the
// timeline area) forces a rowsLayer rebuild, even when Bounds and baseX
// remain unchanged.
//
// This exercises the canReuse/needFull predicate gap (Gap 1) that drumview_
// cache_rows_layer.go's canReuse check originally missed: it only validated
// Bounds W/H, rowOff, baseX — not rowWidth.
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
	originalWidth := g.drum.timelineRect.Dx()
	if originalWidth <= 200 {
		t.Fatalf("test requires timelineRect.Dx() > 200, got %d", originalWidth)
	}

	// Shrink the timeline rect's right edge by 200px without touching Bounds
	// or Min.X. This simulates right-side controls expanding into the timeline
	// area mid-session — the exact scenario that bypassed the old canReuse
	// check, since baseX (= timelineRect.Min.X - Bounds.Min.X) stays put.
	g.drum.timelineRect.Max.X -= 200
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowsLayerGen == gen0 {
		t.Fatalf("expected rowsLayer rebuild after timelineRect.Dx() shrank from %d to %d; gen unchanged at %d",
			originalWidth, g.drum.timelineRect.Dx(), gen0)
	}
	if g.drum.rowsLayerRowWidth != g.drum.timelineRect.Dx() {
		t.Fatalf("rowsLayerRowWidth=%d should equal new timelineRect.Dx()=%d after rebuild",
			g.drum.rowsLayerRowWidth, g.drum.timelineRect.Dx())
	}
}

