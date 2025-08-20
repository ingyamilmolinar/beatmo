//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// TestBPMHoldUpdateLatency ensures that holding the BPM increment button
// never blocks the game update loop even if the audio layer is slow to
// acknowledge tempo changes. It exercises a large number of updates while
// audio.SetBPM sleeps and asserts each call to Update returns promptly.
func TestBPMHoldUpdateLatency(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.playing = true

	// Simulate a sluggish audio layer.
	audio.SetBPMFunc = func(int) { time.Sleep(10 * time.Millisecond) }
	defer func() { audio.SetBPMFunc = func(int) {} }()

	const maxUpdate = 5 * time.Millisecond

	for i := 0; i < 30; i++ {
		g.drum.bpmIncBtn.OnClick()
		start := time.Now()
		if err := g.Update(); err != nil {
			t.Fatalf("update failed: %v", err)
		}
		if dur := time.Since(start); dur > maxUpdate {
			t.Fatalf("update took %v which exceeds %v", dur, maxUpdate)
		}
	}
}
