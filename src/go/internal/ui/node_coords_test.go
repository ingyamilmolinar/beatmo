package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// buildLoopGame builds a 2-node loop a(0,0) -> b(4,0) -> a on row 0 at subdiv 8.
func buildLoopGame(t *testing.T) (g *Game, a, b *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions: %v", err)
	}
	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()
	return g, a, b
}

func TestNodeCircuitPosition(t *testing.T) {
	g, a, b := buildLoopGame(t)

	homeA, occA, rowA, inA := g.nodeCircuitPosition(a.ID)
	if !inA || rowA != 0 {
		t.Fatalf("node a: inCircuit=%v row=%d, want true/0", inA, rowA)
	}
	if homeA != 0 {
		t.Fatalf("node a homeIdx=%d, want 0 (circuit start)", homeA)
	}
	if occA < 1 {
		t.Fatalf("node a occurrences=%d, want >=1", occA)
	}

	homeB, _, _, inB := g.nodeCircuitPosition(b.ID)
	if !inB {
		t.Fatal("node b should be in the circuit")
	}
	if homeB <= homeA {
		t.Fatalf("node b homeIdx=%d should be after node a's %d", homeB, homeA)
	}

	// A disconnected node is off-circuit.
	d := g.tryAddNode(9, 9, model.NodeTypeRegular)
	g.updateBeatInfos()
	if _, _, _, inD := g.nodeCircuitPosition(d.ID); inD {
		t.Fatal("disconnected node should report inCircuit=false")
	}
}

func TestNodeLiveTimingStopped(t *testing.T) {
	g, a, b := buildLoopGame(t)

	// Stopped: next-from-0 (loop start), no prev.
	ta := g.nodeLiveTiming(a.ID)
	if !ta.inCircuit {
		t.Fatal("node a should be in circuit")
	}
	if !ta.nextOK || ta.nextIdx != ta.homeIdx {
		t.Fatalf("stopped next: ok=%v idx=%d, want home %d", ta.nextOK, ta.nextIdx, ta.homeIdx)
	}
	if ta.prevOK {
		t.Fatal("stopped: prev must not be set")
	}

	tb := g.nodeLiveTiming(b.ID)
	if !tb.nextOK || tb.nextIdx != tb.homeIdx {
		t.Fatalf("stopped next for b: idx=%d want home %d", tb.nextIdx, tb.homeIdx)
	}

	// Off-circuit node: no timing.
	d := g.tryAddNode(9, 9, model.NodeTypeRegular)
	g.updateBeatInfos()
	if td := g.nodeLiveTiming(d.ID); td.inCircuit || td.nextOK || td.prevOK {
		t.Fatalf("off-circuit node timing should be empty: %+v", td)
	}
}

func TestNodeLiveTimingPrevFromTimeline(t *testing.T) {
	g, a, _ := buildLoopGame(t)
	g.SetPlaying(true)
	advanceFrames(g, 2)

	// One period past a's home index, so the prev scan [playhead-period, playhead-1] includes homeA.
	period := len(g.beatInfosByRow[0])
	if g.isLoopByRow[0] {
		period = len(g.beatInfosByRow[0]) - g.loopStartByRow[0]
	}
	homeA, _, _, _ := g.nodeCircuitPosition(a.ID)
	g.elapsedBeats = homeA + period
	g.timeline.RecordCommitKind(0, homeA, true, model.NodeTypeRegular,
		timeline.CommitKindPlayback, 256, 256)

	ta := g.nodeLiveTiming(a.ID)
	if !ta.prevOK || ta.prevIdx != homeA {
		t.Fatalf("playing prev: ok=%v idx=%d, want home %d", ta.prevOK, ta.prevIdx, homeA)
	}
}

func TestNodeLiveTimingMemoReuse(t *testing.T) {
	g, a, _ := buildLoopGame(t)
	_ = g.nodeLiveTiming(a.ID)
	if !g.nodeTimingCache.valid || g.nodeTimingCache.id != a.ID {
		t.Fatal("memo should be populated after first call")
	}
	// Second call with nothing changed reuses the cache (playhead unchanged).
	sigBefore := g.nodeTimingCache.pathSig
	_ = g.nodeLiveTiming(a.ID)
	if g.nodeTimingCache.pathSig != sigBefore {
		t.Fatal("memo should be stable across identical calls")
	}
}

func TestFormatNodeCoordLines(t *testing.T) {
	g, a, b := buildLoopGame(t)

	// Exactly on Beat 1, where the fraction is dropped.
	ta := g.nodeLiveTiming(a.ID)
	home := g.formatNodeCoordHome(a.I, a.J, ta)
	if home != "(0, 0) · Beat 1" {
		t.Fatalf("home line = %q, want %q", home, "(0, 0) · Beat 1")
	}

	// Off-circuit node.
	d := g.tryAddNode(9, 9, model.NodeTypeRegular)
	g.updateBeatInfos()
	td := g.nodeLiveTiming(d.ID)
	off := g.formatNodeCoordHome(d.I, d.J, td)
	if off != "(9, 9) · off-circuit" {
		t.Fatalf("off-circuit line = %q, want %q", off, "(9, 9) · off-circuit")
	}
	if g.formatNodeFireLine(td, false) != "" {
		t.Fatal("off-circuit fire line should be empty")
	}

	// Stopped fire line: next only, from position 0 (node a is on Beat 1).
	stopped := g.formatNodeFireLine(ta, false)
	if stopped != "next  Beat 1" {
		t.Fatalf("stopped fire line = %q, want %q", stopped, "next  Beat 1")
	}

	// The raw fraction "k/D", never reduced or off by one: b sits exactly 4/8 into beat 1.
	hb, _, _, _ := g.nodeCircuitPosition(b.ID)
	if hb != 4 {
		t.Fatalf("precondition: b homeIdx = %d, want 4 (grid midpoint)", hb)
	}
	tb := g.nodeLiveTiming(b.ID)
	if got := g.formatNodeCoordHome(b.I, b.J, tb); got != "(4, 0) · Beat 1 · 4/8" {
		t.Fatalf("mid-beat home = %q, want %q", got, "(4, 0) · Beat 1 · 4/8")
	}
}
