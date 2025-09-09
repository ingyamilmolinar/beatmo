//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// TestBPMSpamCoalesces verifies that rapid BPM changes while the audio layer
// is busy are coalesced and do not stall the game loop.
func TestBPMSpamCoalesces(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.playing = true

	block := make(chan struct{})
	var last int
	audio.SetBPMFunc = func(b int) { last = b; <-block }
	defer func() { audio.SetBPMFunc = func(int) {} }()

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
	deadline := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(deadline) {
		if last == g.drum.BPM() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected audio BPM %d, got %d", g.drum.BPM(), last)
}
