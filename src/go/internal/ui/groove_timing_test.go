package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Build a simple 1-beat edge with two nodes: 0 -> 32.
func buildEdge(g *Game) (a, b *uiNode) {
	g.pendingStartRow = 0
	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(32, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	return
}

func captureScheduledWhen(t *testing.T, g *Game) float64 {
	t.Helper()
	whenCh := make(chan float64, 1)
	baseCh := make(chan float64, 1)
	g.playFn = func(id instrument.ID, vol float64, when ...float64) {
		if len(when) > 0 {
			whenCh <- when[0]
			return
		}
		whenCh <- -1
	}
	g.scheduleHook = func(row, idx int) {
		select {
		case baseCh <- audio.Now():
		default:
		}
	}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	w := waitForChan(t, whenCh, 10000)
	if w < 0 {
		return w
	}
	var start float64
	select {
	case start = <-baseCh:
	default:
	}
	return w - start
}

func TestGrooveDelaySchedulesWithinOneSubdivision(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, _ := buildEdge(g)
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.GrooveKind = model.GrooveDelay
		p.GroovePct = 1.0
		g.graph.SetNodeParams(a.ID, p)
	}

	bpm := 120
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	g.audioLookaheadSec = 0
	secPerSub := (60.0 / float64(bpm)) / float64(g.grid.MaxDiv())
	when := captureScheduledWhen(t, g)
	expected := secPerSub
	if diff := when - expected; diff < -0.003 || diff > 0.003 {
		t.Fatalf("delay schedule mismatch: got %.4fs want %.4fs", when, expected)
	}
}

// Rush should schedule earlier than the grid boundary, but not beyond one subdivision.
func TestGrooveRushClampedWithinSubdivision(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, _ := buildEdge(g)
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.GrooveKind = model.GrooveRush
		p.GroovePct = 1.0
		g.graph.SetNodeParams(a.ID, p)
	}

	bpm := 120
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	g.audioLookaheadSec = 0
	secPerSub := (60.0 / float64(bpm)) / float64(g.grid.MaxDiv())
	when := captureScheduledWhen(t, g)
	expected := -secPerSub
	if diff := when - expected; diff < -0.003 || diff > 0.003 {
		t.Fatalf("rush schedule mismatch: got %.4fs want %.4fs", when, expected)
	}
}

// Swing, tuplets and grace notes are per-node delay/rush on otherwise straight nodes; a borrowed offset would
// turn one swung offbeat into a shift of the whole phrase.
func TestGrooveAppliesOnlyToItsOwnNode(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.GrooveKind = model.GrooveDelay
		p.GroovePct = 1.0
		g.graph.SetNodeParams(a.ID, p)
	}
	bpm := 120
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	secPerSub := (60.0 / float64(bpm)) / float64(g.grid.MaxDiv())

	infoA := g.beatInfoAtRow(0, 0)
	if infoA.NodeID != a.ID {
		t.Fatalf("beat 0 resolves to node %d, want %d", infoA.NodeID, a.ID)
	}
	if got := g.nodeNudgeSecondsAt(0, 0, infoA); got < secPerSub-1e-9 || got > secPerSub+1e-9 {
		t.Fatalf("grooved node nudge = %.5fs, want %.5fs", got, secPerSub)
	}

	idxB := -1
	for idx := 1; idx < 64; idx++ {
		if g.beatInfoAtRow(0, idx).NodeID == b.ID {
			idxB = idx
			break
		}
	}
	if idxB < 0 {
		t.Fatal("node b not found on row 0")
	}
	if got := g.nodeNudgeSecondsAt(0, idxB, g.beatInfoAtRow(0, idxB)); got != 0 {
		t.Fatalf("ungrooved node after a grooved one was nudged by %.5fs; groove must not leak to the next node", got)
	}
}
