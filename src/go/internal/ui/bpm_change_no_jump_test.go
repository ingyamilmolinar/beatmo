package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Build a single-row loop where every subdivision step lands on a regular node
// so audio callbacks occur at subdivision resolution. The path goes 0->1->...->N
// then back N->N-1->...->0 to form a loop with uniform 1-subdiv segments.
func buildSubdivLoop(g *Game) {
	div := g.grid.MaxDiv()
	// Create nodes at each subdivision on a horizontal line.
	nodes := make([]*uiNode, div+1)
	for i := 0; i <= div; i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
	}
	// Forward edges
	for i := 0; i < div; i++ {
		g.addEdge(nodes[i], nodes[i+1])
	}
	// Backward edges to close the loop with 1-subdiv segments throughout.
	for i := div; i > 0; i-- {
		g.addEdge(nodes[i], nodes[i-1])
	}
	g.updateBeatInfos()
}

// When changing BPM during playback, inter-event audio intervals must not
// collapse into a burst. This catches a regression where the scheduler jumps
// ahead relative to the original playStart when BPM changes.
func TestBPMChange_NoAudioBurst(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	w, h := 800, 600
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

	buildSubdivLoop(g)

	// Capture scheduled subdivision indices via scheduleHook.
	var idxs []int
	g.scheduleHook = func(row, idx int) {
		if row == 0 {
			idxs = append(idxs, idx)
		}
	}

	// Start at 120 BPM and schedule up to abs=16 in one pass.
	g.SetPlaying(true)
	g.SetAppliedBPMForTest(120)
	g.prevBPM = 120
	g.drum.SetBPM(120)
	for i := 0; i < 8; i++ { // drain burst-capped scheduling
		setPlayStartForAbs(g, 16)
		_ = g.Update()
		if len(g.seqNextIdxs) > 0 && g.seqNextIdxs[0] > 16 {
			break
		}
	}
	lastBefore := -1
	if len(g.seqNextIdxs) > 0 {
		lastBefore = g.seqNextIdxs[0] - 1
	}
	// Align UI playhead so BPM anchoring does not rewind seqNextIdxs to zero.
	if len(g.seqNextIdxs) > 0 {
		if len(g.nextBeatIdxs) != len(g.seqNextIdxs) {
			g.nextBeatIdxs = make([]int, len(g.seqNextIdxs))
		}
		copy(g.nextBeatIdxs, g.seqNextIdxs)
	}

	// Change BPM upward without advancing playStart; the scheduler should not burst.
	idxs = nil
	g.SetAppliedBPMForTest(240)
	g.drum.SetBPM(240)
	setPlayStartForAbs(g, 16)
	_ = g.Update()

	if lastBefore >= 0 && len(idxs) > 0 {
		lastAfter := idxs[len(idxs)-1]
		if lastAfter > lastBefore+2 {
			t.Fatalf("audio burst detected: last idx jumped from %d to %d", lastBefore, lastAfter)
		}
		if len(idxs) > 2 {
			t.Fatalf("audio burst detected: scheduled %d indices after BPM change", len(idxs))
		}
	}
}

// Visually, changing BPM mid-play must not cause large jumps in the tracked
// subdivision index within a single frame.
func TestBPMChange_NoSubdivJump(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
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

	buildSubdivLoop(g)

	g.SetPlaying(true)
	g.SetAppliedBPMForTest(120)
	g.drum.SetBPM(120)
	setPlayStartForAbs(g, 8)
	_ = g.Update()
	before := g.elapsedBeats
	g.drum.SetBPM(240)
	_ = g.Update()
	after := g.elapsedBeats
	if d := after - before; d > 2 { // allow up to 2 subdivs due to frame pacing
		t.Fatalf("subdivision jump after BPM change: %d -> %d (delta=%d)", before, after, d)
	}
}
