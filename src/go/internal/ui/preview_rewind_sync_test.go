package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// expectedFromZero returns the next n visible preview booleans for row starting
// at absolute index base, computed from a clean state (no reliance on live
// counters). It uses the game's internal prediction engine.
func expectedFromZero(g *Game, row, base, n int) []bool {
	if base < 0 {
		base = 0
	}
	horizon := base + n
	g.computePredictions(horizon)
	out := make([]bool, n)
	for i := 0; i < n; i++ {
		idx := base + i
		if row < len(g.predVisibleByRow) && idx < len(g.predVisibleByRow[row]) {
			out[i] = g.predVisibleByRow[row][idx]
		}
	}
	return out
}

// Build a 4-node loop with mixed logic rules to stress prediction/preview.
func buildComplexLoop(g *Game) (a, b, c, d *uiNode) {
	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(1, 0, model.NodeTypeRegular)
	c = g.tryAddNode(2, 0, model.NodeTypeRegular)
	d = g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	// A: every 3rd trigger
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(a.ID, p)
	}
	// B: skip every 2nd trigger
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(b.ID, p)
	}
	// C: trigger if previous triggered
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(c.ID, p)
	}
	// D: trigger if previous skipped
	if n, ok := g.graph.GetNodeByID(d.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(d.ID, p)
	}
	return
}

// TestPreviewRewindMatchesPredictions exposes a desync when the timeline is
// rewound: the drum-row preview uses live counters as seed, which produces
// incorrect visible steps for windows before the current playback index. It
// should instead match predictions computed from zero.
func TestPreviewRewindMatchesPredictions(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a, _, _, _ := buildComplexLoop(g)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	// Advance playback to build up live counters/state.
	g.playing = true
	g.spawnPulseFromRow(0, 0)
	for step := 0; step < 16 && g.activePulse != nil; step++ {
		_ = g.advancePulse(g.activePulse)
	}
	if g.activePulse == nil {
		t.Fatal("expected active pulse after advancing")
	}

	// Rewind preview window before the live next index.
	live := g.nextBeatIdxs[0]
	base := live - 6
	if base < 0 {
		base = 0
	}
	g.drum.Offset = base
	g.refreshDrumRow()
	got := append([]bool(nil), g.drum.Rows[0].Steps...)
	// Expected from clean prediction seeded at absolute zero.
	want := expectedFromZero(g, 0, base, len(got))
	// Compare a reasonable prefix
	n := len(got)
	if n > 12 {
		n = 12
	}
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			t.Fatalf("rewind mismatch at base=%d +%d: got=%v want=%v\npreview=%v\n", base, i, got[i], want[i], got[:n])
		}
	}
}
