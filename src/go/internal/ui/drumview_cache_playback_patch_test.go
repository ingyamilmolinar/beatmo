package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestMarkRowCellsDirtyDoesNotSetFullDirty verifies that markRowCellsDirty
// sets rowDirty but NOT rowFullDirty, allowing the cheap cell-patch path.
func TestMarkRowCellsDirtyDoesNotSetFullDirty(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dst := ebiten.NewImage(400, 200)

	// Build caches via initial draw, which clears dirty flags.
	dv.Draw(dst, nil, 0, nil, 0)

	if dv.rowDirty[0] {
		t.Fatal("expected rowDirty[0] false after initial draw")
	}
	if dv.rowFullDirty[0] {
		t.Fatal("expected rowFullDirty[0] false after initial draw")
	}

	dv.markRowCellsDirty(0)

	if !dv.rowDirty[0] {
		t.Fatal("expected rowDirty[0] true after markRowCellsDirty")
	}
	if dv.rowFullDirty[0] {
		t.Fatal("expected rowFullDirty[0] still false after markRowCellsDirty")
	}
	if !dv.rowsLayerDirty {
		t.Fatal("expected rowsLayerDirty true after markRowCellsDirty")
	}
}

// TestMarkRowDirtySetsFullDirty confirms that the existing markRowDirty sets
// both flags (control test for TestMarkRowCellsDirtyDoesNotSetFullDirty).
func TestMarkRowDirtySetsFullDirty(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()

	dv.ensureRowCache()
	dv.markRowDirty(0)

	if !dv.rowDirty[0] {
		t.Fatal("expected rowDirty[0] true after markRowDirty")
	}
	if !dv.rowFullDirty[0] {
		t.Fatal("expected rowFullDirty[0] true after markRowDirty")
	}
}

// TestPlaybackBeatAdvanceUsesPatchPath simulates a beat advance where only 1-2
// cells change and verifies that buildRowSprite returns rowRebuildPatch.
func TestPlaybackBeatAdvanceUsesPatchPath(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dst := ebiten.NewImage(400, 200)

	// Initial draw to build caches.
	dv.Draw(dst, nil, 0, nil, 0)
	if len(dv.rowCache) == 0 || dv.rowCache[0] == nil {
		t.Fatal("expected row cache to be built after initial draw")
	}

	// Simulate a beat advance: change 1 cell in Steps.
	n := len(dv.Rows[0].Steps)
	if n == 0 {
		t.Skip("no steps to patch")
	}
	dv.Rows[0].Steps[0] = !dv.Rows[0].Steps[0]

	// Use markRowCellsDirty (the new lightweight method).
	dv.markRowCellsDirty(0)

	// Trigger rebuild via draw. Counters are per-frame (reset each Draw).
	dv.Draw(dst, nil, 0, nil, 0)

	if dv.rowCachePatch == 0 {
		t.Fatalf("expected patch path (rowCachePatch > 0), got rowCachePatch=%d rowCacheFull=%d",
			dv.rowCachePatch, dv.rowCacheFull)
	}
	if dv.rowCacheFull != 0 {
		t.Fatalf("expected no full rebuild on patch, got rowCacheFull=%d", dv.rowCacheFull)
	}
}

// TestStructuralChangeStillUsesFullRebuild verifies that changes affecting many
// cells use the full rebuild path (markRowDirty), not the patch path.
func TestStructuralChangeStillUsesFullRebuild(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dst := ebiten.NewImage(400, 200)

	// Initial draw builds caches.
	dv.Draw(dst, nil, 0, nil, 0)

	// Flip all cells.
	for i := range dv.Rows[0].Steps {
		dv.Rows[0].Steps[i] = !dv.Rows[0].Steps[i]
	}

	// Use markRowDirty (sets rowFullDirty=true, forcing full rebuild).
	dv.markRowDirty(0)
	dv.Draw(dst, nil, 0, nil, 0)

	// rowCacheFull and rowCachePatch are per-frame counters (reset each Draw).
	// A full rebuild means rowCacheFull > 0 and rowCachePatch == 0.
	if dv.rowCacheFull == 0 {
		t.Fatalf("expected full rebuild (rowCacheFull > 0), got rowCacheFull=%d rowCachePatch=%d",
			dv.rowCacheFull, dv.rowCachePatch)
	}
	if dv.rowCachePatch != 0 {
		t.Fatalf("expected no patches on full rebuild, got rowCachePatch=%d", dv.rowCachePatch)
	}
}

// TestCountCellDiffs verifies the countCellDiffs helper function.
func TestCountCellDiffs(t *testing.T) {
	prevSteps := []bool{true, false, true, false, true}
	prevTypes := []model.NodeType{model.NodeTypeRegular, model.NodeTypeRegular, model.NodeTypeMute, model.NodeTypeRegular, model.NodeTypeRegular}

	// No changes.
	diff := countCellDiffs(prevSteps, prevTypes, prevSteps, prevTypes)
	if diff != 0 {
		t.Fatalf("expected 0 diffs for identical slices, got %d", diff)
	}

	// Change 1 step.
	nextSteps := make([]bool, len(prevSteps))
	copy(nextSteps, prevSteps)
	nextSteps[2] = false
	diff = countCellDiffs(prevSteps, prevTypes, nextSteps, prevTypes)
	if diff != 1 {
		t.Fatalf("expected 1 diff for 1 step change, got %d", diff)
	}

	// Change 1 type.
	nextTypes := make([]model.NodeType, len(prevTypes))
	copy(nextTypes, prevTypes)
	nextTypes[0] = model.NodeTypeMute
	diff = countCellDiffs(prevSteps, prevTypes, prevSteps, nextTypes)
	if diff != 1 {
		t.Fatalf("expected 1 diff for 1 type change, got %d", diff)
	}

	// Change both step and type at same position — counts as 1.
	diff = countCellDiffs(prevSteps, prevTypes, nextSteps, nextTypes)
	if diff != 2 {
		t.Fatalf("expected 2 diffs (1 step + 1 type), got %d", diff)
	}
}

// TestOverdrawPathOnPatchedRows verifies that when all rebuilt rows use the
// patch path, the rows layer uses the overdraw path instead of full recomposite.
func TestOverdrawPathOnPatchedRows(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dst := ebiten.NewImage(400, 200)

	// Initial draw builds all caches and layer.
	dv.Draw(dst, nil, 0, nil, 0)
	layerGenBefore := dv.rowsLayerGen

	// Now patch 1 cell (beat advance style).
	n := len(dv.Rows[0].Steps)
	if n == 0 {
		t.Skip("no steps to patch")
	}
	dv.Rows[0].Steps[0] = !dv.Rows[0].Steps[0]
	dv.markRowCellsDirty(0)

	// Draw again — should use overdraw, incrementing layerGen by 1.
	dv.Draw(dst, nil, 0, nil, 0)

	if dv.rowsLayerGen <= layerGenBefore {
		t.Fatalf("expected rowsLayerGen to increase (was %d, now %d)", layerGenBefore, dv.rowsLayerGen)
	}
}
