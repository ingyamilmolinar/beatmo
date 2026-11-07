package ui

import (
	"sync/atomic"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Play callbacks keep arriving without long gaps while Update is hammered with length changes.
func TestAudioDecoupledDuringHeavyUIOperations(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Capture play callbacks.
	const total = 80
	var played atomic.Int32
	g.SetPlayFunc(func(id instrument.ID, vol float64, when ...float64) { played.Add(1) })

	// Queue sounds while hammering Update with layout churn.
	for i := 0; i < total; i++ {
		g.queueSoundParams(roster.Snare, 1, 0, 1)
		pressLenInc(t, g.drum)
		_ = g.Update()
		pressLenDec(t, g.drum)
		_ = g.Update()
	}

	waitForCount(t, 10000, total, func() int { return int(played.Load()) })
}
