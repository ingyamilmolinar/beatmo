//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestBPMHoldUpdateLatency ensures that holding the BPM increment button
// never blocks the game update loop even if the audio layer is slow to
// acknowledge tempo changes. It exercises a large number of updates while
// audio.SetBPM sleeps and asserts each call to Update returns promptly.
func TestBPMHoldUpdateLatency(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.SetPlaying(true)

	// Simulate a sluggish audio layer.
	block := make(chan struct{})
	blockClosed := false
	t.Cleanup(func() {
		if !blockClosed {
			close(block)
		}
	})
	prevSetBPM := audio.SetBPMFunc
	audio.SetBPMFuncForTest(func(int) { <-block })
	defer func() { audio.SetBPMFuncForTest(prevSetBPM) }()

	for i := 0; i < 30; i++ {
		g.drum.bpmIncBtn.OnClick()
		done := make(chan struct{})
		go func() {
			_ = g.Update()
			close(done)
		}()
		waitForChan(t, done, 100000)
	}
	close(block)
	blockClosed = true
}
