//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestDrumViewLayerZOrder asserts the exact set of zones + layers
// registered with DrumViewTree and their relative draw order. This is
// the load-bearing invariant behind "z axis is well known and simple"
// — adding, removing, or reordering an entry MUST update both
// drumview_tree.go's Z constant block and this expected list.
//
// The slice is not allowed to drift silently: a new layer that registers
// without a corresponding entry here fails the test, and a removed layer
// that this test still expects also fails.
func TestDrumViewLayerZOrder(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	t.Setenv("BEATMO_DEBUG_LAYOUT", "1") // ensure debug-gated layers also register
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	tree := g.drum.tree
	if tree == nil {
		t.Fatal("drum.tree is nil after Layout")
	}
	got := tree.LayersForTest()
	if len(got) == 0 {
		t.Fatal("drum.tree has no layers registered")
	}

	type expected struct {
		id     string
		zIndex int
	}
	want := []expected{
		// Decorative — background paint goes first so all subsequent
		// layers composite on top. The rack-mask surface sits just above
		// the background so row controls render on top of it, not under.
		{"background", ZBackground},
		{"rack-mask", ZRackMask},
		{"eq-peek", ZEQPeek},
		// Primary content zones, in widget-board reading order.
		{"transport", ZTransport},
		{"timeline", ZTimeline},
		{"row-rack", ZRowRack},
		{"eq-panel", ZEQPanel},
		// Above-content overlays.
		{"transport-pulse", ZTransportPulse},
		{"view-switch", ZViewSwitch},
		{"row-zoom-chips", ZRowZoomChips},
		{"notifications", ZNotifications},
		// Debug-gated chrome (guides under pills so the interactive
		// pill stays visible/clickable when guides are on).
		{"layout-guides", ZLayoutGuides},
		{"layout-pills", ZLayoutPills},
		// Layout-resize hit-test zone — no draw, just lives in the
		// merged slice for input dispatch order.
		{"layout-resize", ZResize},
	}

	if len(got) != len(want) {
		t.Errorf("layer count: got %d want %d", len(got), len(want))
		for i, l := range got {
			t.Logf("  got[%2d] id=%-20q z=%d", i, l.ID(), l.ZIndex())
		}
		for i, w := range want {
			t.Logf("  want[%2d] id=%-20q z=%d", i, w.id, w.zIndex)
		}
		return
	}

	prevZ := -1
	for i, l := range got {
		if l.ZIndex() < prevZ {
			t.Errorf("layer %d (id=%q z=%d) has lower z-index than previous layer (z=%d) — slice not sorted",
				i, l.ID(), l.ZIndex(), prevZ)
		}
		prevZ = l.ZIndex()
		if l.ID() != want[i].id || l.ZIndex() != want[i].zIndex {
			t.Errorf("layer[%d]: got id=%q z=%d, want id=%q z=%d",
				i, l.ID(), l.ZIndex(), want[i].id, want[i].zIndex)
		}
	}
}
