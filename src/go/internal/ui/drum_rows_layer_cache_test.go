package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRowsLayerCacheRebuild verifies the rows composite layer is reused across
// unchanged frames and rebuilt when offset or row content changes.
func TestRowsLayerCacheRebuild(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Minimal setup: one row, small length
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.drum.SetBeatLength(64)
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.Rows[0].Steps[0] = true
	g.drum.SetBounds(image.Rect(0, 300, 800, 600))

	t.Logf("initial length=%d", g.drum.Length)

	rowWidth := g.drum.timelineRect.Dx()
	if rowWidth <= 0 {
		t.Fatalf("timeline width should be > 0")
	}
	pxPerStep := float64(rowWidth) / float64(max1(g.drum.Length))
	if pxPerStep <= 0 {
		t.Fatalf("pixels per step should be > 0")
	}
	maxShift := int(math.Floor(float64(g.drum.rowsLayerPadPx) / pxPerStep))
	if maxShift < 1 {
		maxShift = 1
	}
	if maxShift > g.drum.Length/2 && g.drum.Length > 1 {
		maxShift = g.drum.Length / 2
	}
	shiftSteps := maxShift

	dst := ebiten.NewImage(800, 600)
	// First draw builds caches
	g.drum.Draw(dst, nil, 0, nil, 0)
	gen1 := g.drum.rowsLayerGen
	if gen1 == 0 {
		t.Fatalf("expected rowsLayer to be built")
	}
	fullBytes := g.drum.rowsLayerBytes
	if fullBytes == 0 {
		t.Fatalf("expected initial rowsLayerBytes to be > 0")
	}
	// Second draw with no changes should reuse
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowsLayerGen != gen1 {
		t.Fatalf("rowsLayer should be reused; got gen %d -> %d", gen1, g.drum.rowsLayerGen)
	}
	if g.drum.rowsLayerBytes != 0 {
		t.Fatalf("expected no bytes touched on reuse; got %d", g.drum.rowsLayerBytes)
	}
	// Change offset -> should rebuild layer (but reuse row sprites via shift)
	g.drum.Offset += shiftSteps
	g.drum.markRowsShiftDirty()
	g.drum.Draw(dst, nil, 0, nil, 0)
	gen2 := g.drum.rowsLayerGen
	if gen2 == gen1 {
		t.Fatalf("expected rowsLayer rebuild on offset shift")
	}
	partialBytes := g.drum.rowsLayerBytes
	if partialBytes == 0 {
		t.Fatalf("expected partial rowsLayer reuse to touch bytes")
	}
	t.Logf("rowsLayerPadPx=%d rowWidth=%d pxPerStep=%.2f shiftSteps=%d partialBytes=%d fullBytes=%d", g.drum.rowsLayerPadPx, rowWidth, pxPerStep, shiftSteps, partialBytes, fullBytes)
	if partialBytes >= fullBytes {
		t.Fatalf("expected partial repaint bytes < full repaint bytes, got %d vs %d", partialBytes, fullBytes)
	}
	// Validate the partial repaint width matches the pixel shift.
	absDx := int(math.Round(float64(shiftSteps) * pxPerStep))
	if absDx <= 0 {
		t.Fatalf("expected positive pixel shift; got %d", absDx)
	}
	drawnRows := g.drum.visibleRows()
	if drawnRows > len(g.drum.Rows)-g.drum.rowOffset {
		drawnRows = len(g.drum.Rows) - g.drum.rowOffset
	}
	expectedBytes := int64(drawnRows * absDx * g.drum.rowHeight() * 4)
	if partialBytes != expectedBytes {
		t.Fatalf("expected partial repaint bytes %d, got %d", expectedBytes, partialBytes)
	}
	// Change row content -> should rebuild row sprite and layer
	g.drum.Rows[0].Steps[1] = true
	g.drum.markRowDirty(0)
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowsLayerGen == gen2 {
		t.Fatalf("expected rowsLayer rebuild after row content change")
	}
}
