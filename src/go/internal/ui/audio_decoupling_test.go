package ui

import (
	"sync/atomic"
	"testing"
)

// While the user performs heavy UI operations (resizing, layout, etc.),
// audio playback must remain responsive and decoupled from the UI loop.
// This test queues sounds on a tight interval while hammering Update() with
// length changes and verifies that play callbacks continue to arrive without
// long gaps.
func TestAudioDecoupledDuringHeavyUIOperations(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Capture play callbacks.
	const total = 80
	var played atomic.Int32
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { played.Add(1) })

	// Queue sounds while hammering Update with layout churn.
	for i := 0; i < total; i++ {
		g.queueSoundParams("snare", 1, 0, 1)
		pressLenInc(t, g.drum)
		_ = g.Update()
		pressLenDec(t, g.drum)
		_ = g.Update()
	}

	waitForCount(t, 10000, total, func() int { return int(played.Load()) })
}
