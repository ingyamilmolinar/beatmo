package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestRowCacheRebuildOnOffsetChange ensures that when DrumView auto-tracks and
// changes Offset mid-playback, the row cache is invalidated and rebuilt so the
// visible window continues to update.
func TestRowCacheRebuildOnOffsetChange(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	dst := ebiten.NewImage(800, 200)
	// Build initial drum sprites
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	if len(g.drum.rowCache) == 0 || g.drum.rowCache[0] == nil {
		t.Fatalf("expected initial row cache built")
	}
	first := g.drum.rowCache[0]
	// Simulate tracking to set an offset
	g.drum.TrackBeat(10)
	// Game.Update should observe OffsetChanged and refresh window + mark dirty
	_ = g.Update()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	if g.drum.rowCache[0] == first {
		t.Fatalf("expected row cache to rebuild after offset change")
	}
}
