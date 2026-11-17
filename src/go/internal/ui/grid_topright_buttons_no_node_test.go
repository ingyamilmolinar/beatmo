//go:build test

package ui

import (
	"image"
	"testing"
)

// TestTopRightButtonClusterBlocksNodeCreation pins that the whole top-right
// grid-button cluster (settings gear + folder + logs) — including the gaps
// BETWEEN the buttons and the immediate area around them — is a protected
// no-create zone. A tap that misses a button by a few pixels (landing in a gap,
// just below a button, or on the folder/logs button via the release-time
// gesture-tap path) must NOT fall through to the grid and spawn a node there.
//
// Before the fix, blocksAt() and handleTapInGrid() guarded only the gear rect,
// so a tap in the gear↔folder gap, the folder↔logs gap, just below a button, or
// on the folder/logs buttons themselves created a stray node "below or in
// between the buttons".
func TestTopRightButtonClusterBlocksNodeCreation(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	gear := g.gridHelpButtonRect()
	file := g.gridFileButtonRect()
	logs := g.gridLogsButtonRect()
	if gear.Empty() || file.Empty() || logs.Empty() {
		t.Fatalf("expected all three grid buttons present: gear=%v file=%v logs=%v", gear, file, logs)
	}
	midY := (gear.Min.Y + gear.Max.Y) / 2

	type probe struct {
		name string
		x, y int
	}
	probes := []probe{
		{"gear-file gap", (file.Max.X + gear.Min.X) / 2, midY},
		{"file-logs gap", (logs.Max.X + file.Min.X) / 2, midY},
		{"just below folder", (file.Min.X + file.Max.X) / 2, file.Max.Y + SpaceSM/2},
		{"just below logs", (logs.Min.X + logs.Max.X) / 2, logs.Max.Y + SpaceSM/2},
		{"folder button center", (file.Min.X + file.Max.X) / 2, midY},
		{"logs button center", (logs.Min.X + logs.Max.X) / 2, midY},
	}

	for _, p := range probes {
		// blocksAt is the desktop editor gate (game_update.go only calls
		// handleEditor when !blocksAt). It must claim the whole cluster.
		if !g.blocksAt(p.x, p.y) {
			t.Errorf("%s (%d,%d): blocksAt must be true so the desktop editor cannot create a node here", p.name, p.x, p.y)
		}

		// The mobile/gesture release path: a tap here must create no node.
		before := len(g.graph.Nodes)
		g.handleTapInGrid(p.x, p.y)
		if after := len(g.graph.Nodes); after != before {
			t.Errorf("%s (%d,%d): handleTapInGrid created a node in the protected button zone (nodes %d -> %d)", p.name, p.x, p.y, before, after)
		}
	}

	// Sanity: a tap well clear of the cluster (mid grid, away from the top-right
	// corner) still creates a node — the block zone must not swallow open grid.
	clearX, clearY := 150, 250
	if g.blocksAt(clearX, clearY) {
		t.Fatalf("setup: reference point (%d,%d) should be outside the button zone", clearX, clearY)
	}
	before := len(g.graph.Nodes)
	g.handleTapInGrid(clearX, clearY)
	if len(g.graph.Nodes) <= before {
		t.Errorf("reference tap clear of the buttons (%d,%d) should still create a node (nodes stayed %d)", clearX, clearY, before)
	}
}

// TestTopRightButtonReservedZoneCoversAllThreeButtons pins that the popup-avoid
// reserved zone spans the full cluster (gear + folder + logs), so no grid popup
// can render on/under any of the three buttons. gridTopButtonsReservedRect is
// the zone the sidebar and the button menus avoid.
func TestTopRightButtonReservedZoneCoversAllThreeButtons(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	zone := g.gridTopButtonsReservedRect()
	for _, c := range []struct {
		name string
		r    image.Rectangle
	}{
		{"gear", g.gridHelpButtonRect()},
		{"folder", g.gridFileButtonRect()},
		{"logs", g.gridLogsButtonRect()},
	} {
		if c.r.Empty() {
			t.Fatalf("%s button rect empty", c.name)
		}
		if !c.r.In(zone) {
			t.Errorf("reserved zone %v must fully contain the %s button %v", zone, c.name, c.r)
		}
	}
}
