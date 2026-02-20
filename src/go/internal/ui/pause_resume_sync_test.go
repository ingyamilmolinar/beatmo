package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Verifies that pausing and resuming resumes from the exact position and does
// not jump forward/backward.
func TestPauseResumeKeepsPosition(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	w, h := 640, 480
	g.Layout(w, h)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	defer restore()

	// 1-beat segment
	beat := g.grid.MaxDiv()
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)

	// Start playback
	pressPlay(t, g.drum)
	_ = g.Update()
	// Advance to a deterministic subdivision.
	setPlayStartForAbs(g, g.grid.MaxDiv()+2)
	_ = g.Update()
	// Pause
	pressPlay(t, g.drum)
	_ = g.Update()
	paused := g.elapsedBeats
	// While paused, counters freeze
	advanceFrames(g, 3)
	if g.elapsedBeats != paused {
		t.Fatalf("counters advanced while paused: %d -> %d", paused, g.elapsedBeats)
	}

	// Resume
	pressPlay(t, g.drum)
	_ = g.Update()
	// Immediately after resume, position should be identical
	if g.elapsedBeats != paused {
		t.Fatalf("resume jumped: paused=%d now=%d", paused, g.elapsedBeats)
	}
	// After advancing the timebase, it should advance from the paused position
	setPlayStartForAbs(g, paused+2)
	_ = g.Update()
	if g.elapsedBeats <= paused {
		t.Fatalf("did not advance after resume: %d -> %d", paused, g.elapsedBeats)
	}
}

// Verifies that resuming does not cause a burst of catch-up audio events.
func TestPauseResumeNoBurstAudio(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Tight path with frequent triggers: nodes 0..4 one unit apart, loop back.
	for i := 0; i < 5; i++ {
		_ = g.tryAddNode(i, 0, model.NodeTypeRegular)
	}
	// Edges 0->1->2->3->4->0
	for i := 0; i < 5; i++ {
		a := g.nodes[i]
		b := g.nodes[(i+1)%5]
		g.addEdge(a, b)
	}
	g.updateBeatInfos()
	g.drum.SetBPM(180)
	g.SetAppliedBPMForTest(180)

	// Capture plays
	plays := make(chan struct{}, 1024)
	g.SetPlayFunc(func(string, float64, ...float64) { plays <- struct{}{} })

	// Start and advance to a deterministic point.
	pressPlay(t, g.drum)
	_ = g.Update()
	setPlayStartForAbs(g, g.grid.MaxDiv()+4)
	_ = g.Update()

	// Pause and drain
	pressPlay(t, g.drum)
	_ = g.Update()
	for len(plays) > 0 {
		<-plays
	}
	paused := g.elapsedBeats

	// Simulate a backlog before resume; resume should clamp counters.
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	for i := range g.seqNextIdxs {
		g.seqNextIdxs[i] = 0
		g.nextBeatIdxs[i] = paused
	}

	// Resume and observe a short window for bursts
	pressPlay(t, g.drum)
	_ = g.Update()
	setPlayStartForAbs(g, paused)
	g.scheduleHook = func(row, idx int) {
		if row == 0 {
			plays <- struct{}{}
		}
	}
	g.seqScheduleTime()
	g.scheduleHook = nil
	if len(g.seqNextIdxs) > 0 && g.seqNextIdxs[0] < paused {
		t.Fatalf("resume did not clamp seqNextIdxs: %v (paused=%d)", g.seqNextIdxs, paused)
	}
	if len(plays) != 0 {
		t.Fatalf("burst on resume: scheduled %d events immediately", len(plays))
	}
}
