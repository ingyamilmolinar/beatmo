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

	type expected struct {
		id     string
		zIndex int
	}

	// The drum-view subtree (dv.tree) owns the drum-rows panel chrome.
	// Three entries that used to live here — eq-panel (130), view-switch
	// (150), row-eq-divider (186) — moved ONTO the audio-panel subtree
	// (dv.audioTree); they are asserted below as a separate ordered list.
	wantDrum := []expected{
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
		// Above-content overlays.
		{"transport-pulse", ZTransportPulse},
		{"row-zoom-chips", ZRowZoomChips},
		// (notifications layer removed: the floating toast was replaced by
		// the in-band notification area drawn by the timeline zone + a
		// notif-history portal popup.)
		// Debug-gated chrome (guides under pills so the interactive
		// pill stays visible/clickable when guides are on).
		{"layout-guides", ZLayoutGuides},
		{"layout-pills", ZLayoutPills},
	}

	// The audio-panel subtree (dv.audioTree) owns the EQ/analysis panel
	// region, the mobile Pads↔audio-tab view switcher, and the production
	// divider between the drum-rows panel and the audio panel.
	wantAudio := []expected{
		{"eq-panel", ZEQPanel},
		{"view-switch", ZViewSwitch},
		// Production divider — draw-only, desktop-only, NOT debug-gated.
		{"row-eq-divider", ZRowEQDivider},
		// Layout-resize hit-test zone — no draw, lives in the audio subtree so
		// its divider-pill hit areas (ZResize=200) out-prioritize the eq-panel
		// catch-all (ZEQPanel=130) in the SAME HitIndex. The EQ-boundary pill
		// straddles the panel top edge; keeping resize here makes the divider
		// draggable again post-RootTree.
		{"layout-resize", ZResize},
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
