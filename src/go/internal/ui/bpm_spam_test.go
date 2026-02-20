//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestBPMSpamCoalesces verifies that rapid BPM changes while the audio layer
// is busy are coalesced and do not stall the game loop.
func TestBPMSpamCoalesces(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.SetPlaying(true)

	block := make(chan struct{})
	blockClosed := false
	t.Cleanup(func() {
		if !blockClosed {
			close(block)
		}
	})
	var last int
	prevSetBPM := audio.SetBPMFunc
	audio.SetBPMFuncForTest(func(b int) { last = b; <-block })
	defer func() { audio.SetBPMFuncForTest(prevSetBPM) }()

	// Trigger an initial BPM change that will block inside SetBPMFunc.
	g.drum.bpmIncBtn.OnClick()
	g.Update()

	// Spam many more BPM increments while the audio goroutine is blocked.
	for i := 0; i < 50; i++ {
		g.drum.bpmIncBtn.OnClick()
		g.Update()
	}

	// Unblock audio and allow the BPM goroutine to apply the latest value.
	close(block)
	blockClosed = true
	waitForUpdateCond(t, g, 10000, func() bool { return last == g.drum.BPM() })
	if last != g.drum.BPM() {
		t.Fatalf("expected audio BPM %d, got %d", g.drum.BPM(), last)
	}
}
