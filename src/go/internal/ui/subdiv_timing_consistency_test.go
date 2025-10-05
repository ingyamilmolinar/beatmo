package ui

import (
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// helper: wait a short time running updates
func waitRun(g *Game, d time.Duration) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		_ = g.Update()
		time.Sleep(3 * time.Millisecond)
	}
}

// Build two nodes 1 subdivision apart with bidirectional edges.
func buildBiDirPair(g *Game, step int) (a, b *uiNode) {
	g.pendingStartRow = 0
	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(step, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.pendingStartRow = -1
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	return
}

// Measure average inter-callback interval over N intervals using SetPlayFunc.
func measureIntervals(g *Game, n int) (avg float64, got []float64) {
	times := []time.Time{}
	// Capture scheduling directly to avoid reliance on instrument availability
	// or UI highlight delivery in real-Ebiten runs.
	g.scheduleHook = func(row, idx int) {
		if row == 0 {
			times = append(times, time.Now())
		}
	}
	waitRun(g, 500*time.Millisecond)
	// Restore to avoid side effects on following steps
	g.scheduleHook = nil
	if len(times) < n+2 {
		// start playback; then collect more
		return 0, nil
	}
	sum := 0.0
	got = make([]float64, 0, n)
	for i := 2; i < 2+n && i < len(times); i++ {
		dt := times[i].Sub(times[i-1]).Seconds()
		sum += dt
		got = append(got, dt)
	}
	if len(got) == 0 {
		return 0, nil
	}
	return sum / float64(len(got)), got
}

// Temporal spacing must remain constant across subdivision increases.
func TestSubdivChange8to16KeepsTemporalSpacing(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	g.Layout(800, 600)
	// Freeze input
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set 8: %v", err)
	}
	a, b := buildBiDirPair(g, 1)
	// Use a high-ish BPM to shorten test run while keeping stability.
	g.drum.SetBPM(240)
	waitRun(g, 120*time.Millisecond)
	// Start playback and record baseline
	g.drum.playPressed = true
	waitRun(g, 100*time.Millisecond)
	baseAvg, base := measureIntervals(g, 8)
	if len(base) == 0 {
		t.Fatalf("no baseline events captured")
	}
	// Stop playback
	g.drum.stopPressed = true
	_ = g.Update()

	// Increase subdivisions to 16; nodes should be remapped to 2-step spacing.
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set 16: %v", err)
	}
	// Sanity: coordinates scaled
	if a.I != 0 || b.I != 2 {
		t.Fatalf("node remap mismatch after 8->16: a.I=%d b.I=%d", a.I, b.I)
	}
	// Play again and measure intervals.
	g.drum.playPressed = true
	waitRun(g, 100*time.Millisecond)
	newAvg, vals := measureIntervals(g, 8)
	if len(vals) == 0 {
		t.Fatalf("no post-change events captured")
	}

	// Expected interval corresponds to 1/8th note, unchanged across subdiv.
	bpm := 240.0
	want := (60.0 / bpm) / 8.0
	tol := 0.006 // 6ms tolerance
	if math.Abs(baseAvg-want) > tol {
		t.Fatalf("baseline interval off: got=%.4fs want=%.4fs", baseAvg, want)
	}
	if math.Abs(newAvg-want) > tol {
		t.Fatalf("post-change interval off: got=%.4fs want=%.4fs", newAvg, want)
	}
	// Relative comparison: new and base within tolerance
	if math.Abs(newAvg-baseAvg) > tol {
		t.Fatalf("interval changed after subdiv: base=%.4fs new=%.4fs", baseAvg, newAvg)
	}
}

// Temporal spacing must remain constant across subdivision decreases when nodes align.
func TestSubdivChange16to8KeepsTemporalSpacing(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	g.Layout(800, 600)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set 16: %v", err)
	}
	a, b := buildBiDirPair(g, 2) // aligned for 16->8 (multiples of 2)
	g.drum.SetBPM(180)
	waitRun(g, 120*time.Millisecond)
	g.drum.playPressed = true
	waitRun(g, 100*time.Millisecond)
	baseAvg, base := measureIntervals(g, 8)
	if len(base) == 0 {
		t.Fatalf("no baseline events captured")
	}
	g.drum.stopPressed = true
	_ = g.Update()
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set 8: %v", err)
	}
	if a.I != 0 || b.I != 1 {
		t.Fatalf("node remap mismatch after 16->8: a.I=%d b.I=%d", a.I, b.I)
	}
	g.drum.playPressed = true
	waitRun(g, 100*time.Millisecond)
	newAvg, vals := measureIntervals(g, 8)
	if len(vals) == 0 {
		t.Fatalf("no post-change events captured")
	}
	bpm := 180.0
	want := (60.0 / bpm) / 8.0
	tol := 0.008
	if math.Abs(baseAvg-want) > tol {
		t.Fatalf("baseline interval off: got=%.4fs want=%.4fs", baseAvg, want)
	}
	if math.Abs(newAvg-want) > tol {
		t.Fatalf("post-change interval off: got=%.4fs want=%.4fs", newAvg, want)
	}
}
