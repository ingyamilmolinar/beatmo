//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// newLevelsReentryZone builds an EQPanelZone whose analyzer state is driven
// by the returned setLoud toggle. Loud => master at -3 dB peak / -9 dB RMS;
// silent => master at the analyzer dB floor (what the live meters report once
// audio has stopped and the master ring has drained to silence).
//
// The returned draw function advances uiAnimFrame before each Draw — exactly
// what DrumView.Draw does once per frame in production — so the Levels-tab
// re-entry detection (which keys off the global frame counter) is exercised
// faithfully: continuous viewing sees a one-frame gap, a hidden interval sees
// a larger gap.
func newLevelsReentryZone(t *testing.T) (z *EQPanelZone, draw func(), setLoud func(bool)) {
	t.Helper()
	savedFrame := uiAnimFrame
	t.Cleanup(func() { uiAnimFrame = savedFrame })

	loud := false
	cb := EQCallbacks{
		ActiveRows: func() []*DrumRow { return nil },
		AnalyzerState: func() *analyzer.State {
			m := analyzer.ChannelMetrics{ID: "master", Name: "Master"}
			if loud {
				m.PeakDB, m.RMSDB, m.Active = -3, -9, true
			} else {
				m.PeakDB, m.RMSDB = analyzerDBFloor, analyzerDBFloor
			}
			return &analyzer.State{Master: m}
		},
	}
	z = NewEQPanelZone(cb)
	tree := registerEQZone(z, image.Rect(0, 400, 800, 600))
	tree.Update() // initial layout sets z.rect
	screen := ebiten.NewImage(800, 600)
	return z, func() {
		uiAnimFrame++
		z.Draw(screen)
	}, func(v bool) { loud = v }
}

// TestLevelsTab_ReflectsLiveSilenceOnReentry reproduces the reported bug: a
// user watches the Levels tab during playback, switches to another tab where
// audio then stops completely, and on returning to the Levels tab sees a high
// meter that slowly drains to zero instead of immediately showing silence.
//
// Root cause: the peak-hold latch is only ticked while the Levels tab is
// drawn, so a loud value held during playback freezes while the tab is hidden
// and then decays from that stale value on return. The meter must instead
// reflect the live (silent) audio immediately.
func TestLevelsTab_ReflectsLiveSilenceOnReentry(t *testing.T) {
	defer noInputForTest()()
	z, draw, setLoud := newLevelsReentryZone(t)

	// 1. On the Levels tab with loud audio: master peak-hold rises high.
	z.SetActiveTab(TabMeters)
	setLoud(true)
	for i := 0; i < 30; i++ {
		draw()
	}
	if got := z.levelsLatches.Get("main").PeakHoldDB; got < -10 {
		t.Fatalf("setup: expected a loud master peak-hold (>= -10 dB), got %v dB", got)
	}

	// 2. Switch away to Wave; audio then stops completely.
	z.SetActiveTab(TabWave)
	for i := 0; i < 10; i++ {
		draw()
	}
	setLoud(false)

	// 3. Return to the Levels tab. The first drawn frame must reflect live
	//    silence, not a stale loud value held since the tab was last visible.
	z.SetActiveTab(TabMeters)
	draw()

	if got := z.levelsLatches.Get("main").PeakHoldDB; got > analyzerDBFloor+1 {
		t.Fatalf("Levels meter stale after return: master PeakHoldDB=%v dB, want ~%v dB (live silence)", got, analyzerDBFloor)
	}
}

// TestLevelsTab_ReflectsLiveLevelOnReentryWhileLouder guards the symmetric
// case: returning to the Levels tab must also snap UP to a louder live level,
// not linger at a stale quiet value. This keeps the fix from degenerating into
// "always show silence on entry" — the requirement is real-time accuracy.
func TestLevelsTab_ReflectsLiveLevelOnReentryWhileLouder(t *testing.T) {
	defer noInputForTest()()
	z, draw, setLoud := newLevelsReentryZone(t)

	// Levels tab seen while silent: latch seeded near the floor.
	z.SetActiveTab(TabMeters)
	setLoud(false)
	for i := 0; i < 10; i++ {
		draw()
	}

	// Switch away; audio becomes loud while the tab is hidden.
	z.SetActiveTab(TabWave)
	for i := 0; i < 10; i++ {
		draw()
	}
	setLoud(true)

	// Return: first frame must already reflect the live loud level.
	z.SetActiveTab(TabMeters)
	draw()

	if got := z.levelsLatches.Get("main").PeakHoldDB; got < -10 {
		t.Fatalf("Levels meter did not track live loud level after return: master PeakHoldDB=%v dB, want ~-3 dB", got)
	}
}

// TestLevelsTab_PeakHoldBallisticsSurviveContinuousViewing guards the
// re-entry fix from over-firing: while the Levels tab stays visible, a brief
// transient must remain held (peak-hold ballistics) across subsequent quiet
// frames rather than being reset to the live value every frame. The re-entry
// guard must only fire when the tab was actually hidden between frames.
func TestLevelsTab_PeakHoldBallisticsSurviveContinuousViewing(t *testing.T) {
	defer noInputForTest()()
	z, draw, setLoud := newLevelsReentryZone(t)

	z.SetActiveTab(TabMeters)

	// One loud transient frame...
	setLoud(true)
	draw()
	// ...followed by several quiet frames, all while the tab stays visible.
	setLoud(false)
	for i := 0; i < 3; i++ {
		draw()
	}

	// The peak-hold should still be pinned well above the silence floor
	// (sticky hold), proving the latch was NOT reset on these frames.
	if got := z.levelsLatches.Get("main").PeakHoldDB; got <= analyzerDBFloor+1 {
		t.Fatalf("peak-hold ballistics lost during continuous viewing: PeakHoldDB=%v dB, want held near -3 dB", got)
	}
}
