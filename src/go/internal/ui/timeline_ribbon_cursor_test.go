package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// ribbonCursorCapture records the timeline ribbon's playback cursor X and the
// view-rect (the highlighted "active drum view" window) X bounds by
// intercepting drawRect during a full Game.Draw. Both are produced inside
// (*TimelineZone).drawTimelineBar, keyed by their theme colors.
type ribbonCursorCapture struct {
	cursorX        int
	cursorSeen     bool
	viewX0, viewX1 int
	viewSeen       bool
}

func captureRibbonCursor(t *testing.T, g *Game, screen *ebiten.Image) ribbonCursorCapture {
	t.Helper()
	var c ribbonCursorCapture
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, col color.Color, filled bool) {
		switch {
		case col == colTimelineCursor && filled:
			c.cursorX = (r.Min.X + r.Max.X) / 2
			c.cursorSeen = true
		case col == colTimelineView && filled:
			c.viewX0, c.viewX1 = r.Min.X, r.Max.X
			c.viewSeen = true
		}
		orig(dst, r, col, filled)
	}
	defer func() { drawRect = orig }()
	g.Draw(screen)
	return c
}

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

// TestRibbonCursorStaysInViewRectWhileFollowing asserts that, while playback
// auto-follow is on, the ribbon's playback cursor line never falls outside the
// view-rect (the highlighted active-drum-view window) as playback advances.
//
// Regression: the early-session "float the cursor at its true position" branch
// used the wrong fraction threshold, so for a mid-range band of beats the
// cursor pinned at frac*barWidth while the window was still pinned at zero and
// the view-rect tracked the true (much earlier) position — leaving the cursor
// line stranded to the right of the active-drum-view window.
func TestRibbonCursorStaysInViewRectWhileFollowing(t *testing.T) {
	w, h := 1000, 700
	g, cleanup := newRibbonCursorGame(t, w, h)
	defer cleanup()
	screen := ebiten.NewImage(w, h)
	div := float64(g.grid.MaxDiv())

	// Sweep playback position across the whole early-to-steady-state range,
	// including the band that used to strand the cursor (~50..118 beats for a
	// 674px desktop bar at 4 px/beat and frac=0.70).
	for _, beat := range []float64{5, 20, 40, 60, 75, 90, 100, 120, 150, 200} {
		setPlayStartForAbsFloat(g, beat*div)
		_ = g.Update()
		c := captureRibbonCursor(t, g, screen)
		if !c.cursorSeen || !c.viewSeen {
			t.Fatalf("beat=%.0f: missing draw (cursorSeen=%v viewSeen=%v)", beat, c.cursorSeen, c.viewSeen)
		}
		if c.cursorX < c.viewX0 || c.cursorX > c.viewX1 {
			t.Errorf("beat=%.0f displayBeat=%.2f: cursor x=%d outside view-rect [%d..%d] (offset=%d)",
				beat, g.displayBeat(), c.cursorX, c.viewX0, c.viewX1, g.drum.Offset)
		}
	}
}

// TestRibbonCursorInViewRectAfterBPMChange reproduces the user-reported bug:
// changing BPM during playback must keep the global-timeline playback line
// inside the active-drum-view window. The drum view and timeline must adapt to
// the new tempo without stranding the cursor outside the view-rect.
func TestRibbonCursorInViewRectAfterBPMChange(t *testing.T) {
	w, h := 1000, 700
	g, cleanup := newRibbonCursorGame(t, w, h)
	defer cleanup()
	screen := ebiten.NewImage(w, h)
	div := float64(g.grid.MaxDiv())

	// Establish playback well into the session (a beat in the affected band).
	setPlayStartForAbsFloat(g, 80*div)
	_ = g.Update()

	// User raises the tempo mid-playback. Mirror the real flow: the UI BPM
	// changes immediately (re-anchor path) and the engine ACK applies a frame
	// later, then more frames elapse at the new tempo.
	g.drum.SetBPM(240)
	_ = g.Update()
	g.SetAppliedBPMForTest(240)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}

	c := captureRibbonCursor(t, g, screen)
	if !c.cursorSeen || !c.viewSeen {
		t.Fatalf("missing draw after BPM change (cursorSeen=%v viewSeen=%v)", c.cursorSeen, c.viewSeen)
	}
	if c.cursorX < c.viewX0 || c.cursorX > c.viewX1 {
		t.Errorf("after BPM change: cursor x=%d outside view-rect [%d..%d] (displayBeat=%.2f offset=%d)",
			c.cursorX, c.viewX0, c.viewX1, g.displayBeat(), g.drum.Offset)
	}
}
