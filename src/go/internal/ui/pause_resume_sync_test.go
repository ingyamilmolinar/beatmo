package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Verifies that pausing and resuming resumes from the exact position and does
// not jump forward/backward.
func TestPauseResumeKeepsPosition(t *testing.T) {
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

	// 1-beat segment
	beat := g.grid.MaxDiv()
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	g.drum.SetBPM(120)
	// Apply BPM
	until := time.Now().Add(12 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}

	// Start playback
	g.drum.playPressed = true
	// Let it advance for a bit
	until = time.Now().Add(24 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}
	// Pause
	g.drum.playPressed = true
	_ = g.Update()
	paused := g.elapsedBeats
	// While paused, counters freeze
	until = time.Now().Add(12 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}
	if g.elapsedBeats != paused {
		t.Fatalf("counters advanced while paused: %d -> %d", paused, g.elapsedBeats)
	}

	// Resume
	g.drum.playPressed = true
	_ = g.Update()
	// Immediately after resume, position should be identical
	if g.elapsedBeats != paused {
		t.Fatalf("resume jumped: paused=%d now=%d", paused, g.elapsedBeats)
	}
	// After some time, it should advance from the paused position
	until = time.Now().Add(24 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}
	if g.elapsedBeats <= paused {
		t.Fatalf("did not advance after resume: %d -> %d", paused, g.elapsedBeats)
	}
}

// Verifies that resuming does not cause a burst of catch-up audio events.
func TestPauseResumeNoBurstAudio(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
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
	// Apply BPM
	until := time.Now().Add(30 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}

	// Capture plays
	plays := make(chan struct{}, 1024)
	g.SetPlayFunc(func(string, float64, ...float64) { plays <- struct{}{} })

	// Start and run briefly
	g.drum.playPressed = true
	until = time.Now().Add(60 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	// Pause and drain
	g.drum.playPressed = true
	_ = g.Update()
	for len(plays) > 0 {
		<-plays
	}

	// Resume and observe a short window for bursts
	g.drum.playPressed = true
	start := time.Now()
	for time.Since(start) < 20*time.Millisecond {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}
	// Expected events in 20ms ≈ div * bpm/60 * window; allow small tolerance.
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	expected := int(float64(div) * float64(g.bpm) / 60.0 * 0.02)
	if len(plays) > expected+1 {
		t.Fatalf("burst on resume: %d events in 20ms (expected ~%d)", len(plays), expected)
	}
}
