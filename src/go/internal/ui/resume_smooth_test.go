package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Ensures highlight index and drum offset do not jump on resume and advance
// smoothly in small increments.
func TestResumeHighlightSmoothNoJump(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
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

	// Simple 1-beat segment
	beat := g.grid.MaxDiv()
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	g.drum.SetBPM(120)
	until := time.Now().Add(12 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}

	g.drum.playPressed = true
	until = time.Now().Add(24 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}

	// Pause, record state
	g.drum.playPressed = true
	_ = g.Update()
	pausedIdx := g.elapsedBeats
	pausedOff := g.drum.Offset

	// Resume, ensure no jump
	g.drum.playPressed = true
	_ = g.Update()
	if g.elapsedBeats != pausedIdx {
		t.Fatalf("resume jumped: idx %d -> %d", pausedIdx, g.elapsedBeats)
	}
	if g.drum.Offset != pausedOff {
		t.Fatalf("offset changed on resume: %d -> %d", pausedOff, g.drum.Offset)
	}

	// Advance a few frames; ensure highlight progresses by at most 1/subdiv per update
	last := g.elapsedBeats
	for i := 0; i < 5; i++ {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
		cur := g.elapsedBeats
		if cur < last {
			t.Fatalf("highlight moved backwards: %d -> %d", last, cur)
		}
		if cur-last > 2 { // allow up to 2 subdiv in case of timer tick
			t.Fatalf("highlight jumped too far: %d -> %d", last, cur)
		}
		last = cur
	}
}
