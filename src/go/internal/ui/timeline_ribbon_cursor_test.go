package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Deterministic input stubs, playback started at BPM 120.
func newRibbonCursorGame(t *testing.T, w, h int) (*Game, func()) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	g.Layout(w, h)
	buildSubdivLoop(g)
	g.SetPlaying(true)
	g.SetAppliedBPMForTest(120)
	g.prevBPM = 120
	g.bpm = 120
	g.drum.SetBPM(120)
	cleanup := func() {
		restore()
		g.CloseForTest()
	}
	return g, cleanup
}
