package ui

import (
	"testing"
	"time"

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
	g.SetUseSequencerForTest(true)
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

	// Capture audio callback times.
	var times []time.Time
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { times = append(times, time.Now()) })

	// Start at 120 BPM.
	g.drum.SetBPM(120)
	// Allow initial BPM apply.
	until := time.Now().Add(40 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(3 * time.Millisecond)
	}

	// Start playback and run briefly to collect some events.
	g.drum.playPressed = true
	run1 := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(run1) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	// Change BPM upward significantly to magnify any burst if present.
	g.drum.SetBPM(240)
	// Let updates flow while capturing more events.
	run2 := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(run2) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	if len(times) < 8 {
		t.Fatalf("insufficient audio events recorded: %d", len(times))
	}

	// Expected interval between subdivision callbacks at 240 BPM.
	// One subdiv interval in seconds = 60 / (BPM * div).
	div := float64(g.grid.MaxDiv())
	expected := 60.0 / (240.0 * div)
	// Tolerate scheduling jitter; flag a burst if an interval drops below 60% of expected.
	thresh := expected * 0.6
	for i := 1; i < len(times); i++ {
		dt := times[i].Sub(times[i-1]).Seconds()
		if dt < thresh {
			t.Fatalf("audio burst detected: interval=%.4fms < %.4fms", dt*1000, thresh*1000)
		}
	}
}

// Visually, changing BPM mid-play must not cause large jumps in the tracked
// subdivision index within a single frame.
func TestBPMChange_NoSubdivJump(t *testing.T) {
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

	buildSubdivLoop(g)

	g.drum.SetBPM(120)
	until := time.Now().Add(30 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	g.drum.playPressed = true
	start := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(start) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	// Record current index, change BPM, then run one Update and verify the
	// delta is small (no large jump within a single frame).
	before := g.elapsedBeats
	g.drum.SetBPM(240)
	_ = g.Update()
	after := g.elapsedBeats
	if d := after - before; d > 2 { // allow up to 2 subdivs due to frame pacing
		t.Fatalf("subdivision jump after BPM change: %d -> %d (delta=%d)", before, after, d)
	}
}
