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
//
// NOTE: "grid-sidebar" IS a GridTree layer at z=GZSidebar. The node sidebar
// is a compact, node-anchored card that is clamped to the grid pane (it can
// never spill past the split line into the drum pane), so it renders as an
// ordinary grid-local layer rather than through the global OverlayPortal.
// It composites over the coord badge because GZSidebar > GZCoordBadge — see
// TestCoordBadgeDrawnBeforeSidebarPanel / TestCoordBadgeBelowSidebarZ in
// grid_coord_badge_zorder_test.go for that invariant.
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
		{"grid-guide", GZGuide},
		{"grid-coord-badge", GZCoordBadge},
		{"grid-group-badges", GZGroupBadges},
		{"grid-move-mode", GZMoveMode},
		{"grid-connect-mode", GZConnectMode},
		{"grid-mode-banner", GZModeBanner},
		{"grid-marquee", GZMarquee},
		{"grid-longpress", GZLongPress},
		{"grid-sidebar", GZSidebar},
		{"grid-group-menu", GZGroupMenu},
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
