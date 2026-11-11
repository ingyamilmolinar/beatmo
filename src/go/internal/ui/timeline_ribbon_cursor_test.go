package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// newRibbonCursorGame builds a Game with deterministic input stubs, laid out
// at the given size, with playback started at BPM 120. Shared by ribbon/timeline
// rendering tests in this package (see vice_city_drumpane_test.go).
//
// The two tests that used to live in this file
// (TestRibbonCursorStaysInViewRectWhileFollowing and
// TestRibbonCursorInViewRectAfterBPMChange) asserted the playback cursor
// never strayed outside the ribbon's "view rect" — the pattern-window
// highlight that lived inside the old fixed-beats-per-pixel sliding-window
// ribbon. Task 6 removed the view rect entirely and replaced the sliding
// window with a slim whole-song progress bar (see (*TimelineZone).drawTimelineBar),
// so that invariant no longer applies. See
// TestTimelineRibbon_CursorAtWholeSongFraction in
// timeline_zone_long_session_test.go for the replacement cursor-position
// invariant.
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
