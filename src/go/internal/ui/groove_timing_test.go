package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
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
	g.playFn = func(id string, vol float64, when ...float64) {
		if len(when) > 0 {
			whenCh <- when[0]
			return
		}
		whenCh <- -1
	}
	g.scheduleHook = func(row, idx int) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	start := audio.Now()
	_ = g.Update()
	w := waitForChan(t, whenCh, 10000)
	if w < 0 {
		return w
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
		p.GrooveKind = "delay"
		p.GroovePct = 1.0
		g.graph.SetNodeParams(a.ID, p)
	}

	bpm := 120
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	look := g.runtimeAudioLookahead()
	secPerSub := (60.0 / float64(bpm)) / float64(g.grid.MaxDiv())
	when := captureScheduledWhen(t, g)
	expected := look + secPerSub
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
		p.GrooveKind = "rush"
		p.GroovePct = 1.0
		g.graph.SetNodeParams(a.ID, p)
	}

	bpm := 120
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	look := g.runtimeAudioLookahead()
	secPerSub := (60.0 / float64(bpm)) / float64(g.grid.MaxDiv())
	when := captureScheduledWhen(t, g)
	expected := look - secPerSub
	if diff := when - expected; diff < -0.003 || diff > 0.003 {
		t.Fatalf("rush schedule mismatch: got %.4fs want %.4fs", when, expected)
	}
}
