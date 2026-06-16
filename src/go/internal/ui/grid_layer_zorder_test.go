//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestGridLayerZOrder pins the grid pane's Layer/Zone draw order. Adding,
// removing, or reordering a grid participant MUST update both the GZ*
// constants in grid_tree.go and this expected list. Mirrors
// TestDrumViewLayerZOrder for the top pane.
func TestGridLayerZOrder(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	got := g.gridTree.LayersForTest()
	type expected struct {
		id string
		z  int
	}
	want := []expected{
		{"grid-bg", GZBackground},
		{"grid-edges", GZEdges},
		{"grid-nodes", GZCanvas},
		{"grid-pulses", GZPulses},
		{"grid-coord-badge", GZCoordBadge},
		{"grid-move-mode", GZMoveMode},
		{"grid-connect-mode", GZConnectMode},
		{"grid-longpress", GZLongPress},
		{"grid-move-confirm", GZMoveConfirm},
		{"grid-sidebar", GZSidebar},
		{"grid-cursor-label", GZCursorLabel},
		{"grid-help-button", GZGridHelpButton},
	}
	if len(got) != len(want) {
		for i, l := range got {
			t.Logf("got[%2d] id=%-20q z=%d", i, l.ID(), l.ZIndex())
		}
		t.Fatalf("grid layer count: got %d want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ID() != w.id || got[i].ZIndex() != w.z {
			t.Errorf("grid layer[%d]: got (%q,%d) want (%q,%d)", i, got[i].ID(), got[i].ZIndex(), w.id, w.z)
		}
	}
}
