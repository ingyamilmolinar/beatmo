//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Holding BPM+ never blocks Update even when audio.SetBPM is slow.
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
	prevSetBPM := audio.BPMFuncForTest()
	audio.SetBPMFuncForTest(func(int) { <-block })
	defer func() { audio.SetBPMFuncForTest(prevSetBPM) }()

	for i := 0; i < 30; i++ {
		g.drum.bpmIncBtn().OnClick()
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
