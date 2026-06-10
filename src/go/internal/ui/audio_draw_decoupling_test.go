package ui

import (
	"sync/atomic"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Ensure that heavy DrumView.Draw work (many rows, long timelines) does not
// stall audio queuing. We queue sounds rapidly while forcing repeated Draws
// and assert gaps remain bounded.
func TestAudioDecoupledDuringHeavyDraw(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	w, h := 800, 600
	g.Layout(w, h)

	// Make the drum view heavier: add rows and increase length toward width.
	for len(g.drum.Rows) < 12 {
		g.drum.AddRow()
	}
	// Clamp length to the visible timeline width via control logic; do a few
	// increments to exercise path/step rebuild and layout updates.
	for i := 0; i < 20; i++ {
		pressLenInc(t, g.drum)
		_ = g.Update()
	}

	// Capture play callbacks.
	const total = 64
	var played atomic.Int32
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { played.Add(1) })

	// Hammer Draw while queueing sounds.
	img := ebiten.NewImage(w, h)
	for i := 0; i < total; i++ {
		g.queueSoundParams("snare", 1, 0, 1)
		g.drawDrumPane(img)
	}

	waitForCount(t, 10000, total, func() int { return int(played.Load()) })
}
