package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// For large length (steps per pixel ~1), a shift of 1 step is a 1px shift and
// should use incremental update (no generation bump) within pad.
func TestRowCacheOffsetSmallShiftReuse(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	// Make each step ~1px wide so a +1 offset is a tiny pixel shift
	dv.SetLength(dv.timelineRect.Dx())
	dst := ebiten.NewImage(400, 200)
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if len(dv.rowCacheGen) == 0 {
		t.Fatalf("row cache gen missing")
	}
	gen := dv.rowCacheGen[0]
	// Small shift: +1 step
	dv.Offset++
	dv.markRowsShiftDirty()
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if dv.rowCacheGen[0] != gen {
		t.Fatalf("expected incremental reuse (no rebuild), gen %d -> %d", gen, dv.rowCacheGen[0])
	}
}

// With a small pad, a 1-step shift at default length should exceed pad and
// trigger a rebuild (generation bump).
func TestRowCacheOffsetLargeShiftRebuild(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dv.rowCachePadPx = 2 // tiny pad to force rebuilds
	dst := ebiten.NewImage(400, 200)
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	gen := dv.rowCacheGen[0]
	dv.Offset++
	dv.markRowsShiftDirty()
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if dv.rowCacheGen[0] == gen {
		t.Fatalf("expected rebuild (gen bump) after large effective shift; gen still %d", gen)
	}
}
