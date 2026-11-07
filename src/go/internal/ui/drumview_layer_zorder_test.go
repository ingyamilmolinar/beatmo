//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The exact registration set and order: adding, removing or reordering an entry must update drumview_tree.go
// and this list together.
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

	type expected struct {
		id     string
		zIndex int
	}

	// eq-panel, view-switch and row-eq-divider live on the audio subtree, asserted below.
	wantDrum := []expected{
		// The rack mask sits just above the background so row controls render on top of it.
		{"background", ZBackground},
		{"rack-mask", ZRackMask},
		{"eq-peek", ZEQPeek},
		// Primary content zones, in widget-board reading order.
		{"transport", ZTransport},
		{"timeline", ZTimeline},
		{"row-rack", ZRowRack},
		// Above-content overlays.
		{"transport-pulse", ZTransportPulse},
		{"row-zoom-chips", ZRowZoomChips},
		// Guides under pills, so the interactive pill stays visible when guides are on.
		{"layout-guides", ZLayoutGuides},
		{"layout-pills", ZLayoutPills},
	}

	wantAudio := []expected{
		{"eq-panel", ZEQPanel},
		{"view-switch", ZViewSwitch},
		// Production divider — draw-only, desktop-only, NOT debug-gated.
		{"row-eq-divider", ZRowEQDivider},
		// Here so its divider pills (ZResize) outrank the panel catch-all in the SAME HitIndex.
		{"layout-resize", ZResize},
		// Topmost, for when the grid pane is not what the mobile user sees.
		{"drumview-toast", ZDrumToast},
	}

	assertTreeLayers := func(name string, tree *DrumViewTree, want []expected) {
		t.Helper()
		if tree == nil {
			t.Fatalf("drum.%s is nil after Layout", name)
		}
		got := tree.LayersForTest()
		if len(got) == 0 {
			t.Fatalf("drum.%s has no layers registered", name)
		}
		if len(got) != len(want) {
			t.Errorf("%s layer count: got %d want %d", name, len(got), len(want))
			for i, l := range got {
				t.Logf("  %s got[%2d] id=%-20q z=%d", name, i, l.ID(), l.ZIndex())
			}
			for i, w := range want {
				t.Logf("  %s want[%2d] id=%-20q z=%d", name, i, w.id, w.zIndex)
			}
			return
		}

		prevZ := -1
		for i, l := range got {
			if l.ZIndex() < prevZ {
				t.Errorf("%s layer %d (id=%q z=%d) has lower z-index than previous layer (z=%d) — slice not sorted",
					name, i, l.ID(), l.ZIndex(), prevZ)
			}
			prevZ = l.ZIndex()
			if l.ID() != want[i].id || l.ZIndex() != want[i].zIndex {
				t.Errorf("%s layer[%d]: got id=%q z=%d, want id=%q z=%d",
					name, i, l.ID(), l.ZIndex(), want[i].id, want[i].zIndex)
			}
		}
	}

	assertTreeLayers("tree", g.drum.tree, wantDrum)
	assertTreeLayers("audioTree", g.drum.audioTree, wantAudio)
}
