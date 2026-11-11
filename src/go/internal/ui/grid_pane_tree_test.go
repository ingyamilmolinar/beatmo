//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestGameHasGridTreeAfterNew(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	if g.gridTree == nil {
		t.Fatal("g.gridTree must be constructed in New()")
	}
}
