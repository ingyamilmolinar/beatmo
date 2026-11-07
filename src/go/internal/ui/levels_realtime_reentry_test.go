//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Loud: master -3 dB peak / -9 dB RMS; silent: the dB floor. draw advances uiAnimFrame as DrumView.Draw does, so
// re-entry detection sees a one-frame gap while viewed and a larger one after hiding.
func newLevelsReentryZone(t *testing.T) (z *EQPanelZone, draw func(), setLoud func(bool)) {
	t.Helper()
	savedFrame := uiAnimFrame
	t.Cleanup(func() { uiAnimFrame = savedFrame })

	loud := false
	cb := EQCallbacks{
		ActiveRows: func() []*DrumRow { return nil },
		AnalyzerState: func() *analyzer.State {
			m := analyzer.ChannelMetrics{ID: channel.Master, Name: "Master"}
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

// The latch only ticks while the tab draws, so a loud value froze while hidden and drained slowly on return;
// the meter shows the live silence at once.
func TestLevelsTab_ReflectsLiveSilenceOnReentry(t *testing.T) {
	defer noInputForTest()()
	z, draw, setLoud := newLevelsReentryZone(t)

	// 1. On the Levels tab with loud audio: master peak-hold rises high.
	z.SetActiveTab(TabMeters)
	setLoud(true)
	for i := 0; i < 30; i++ {
		draw()
	}
	if got := z.levelsLatches.Get(channel.Master).PeakHoldDB; got < -10 {
		t.Fatalf("setup: expected a loud master peak-hold (>= -10 dB), got %v dB", got)
	}

	// 2. Switch away to Wave; audio then stops completely.
	z.SetActiveTab(TabWave)
	for i := 0; i < 10; i++ {
		draw()
	}
	setLoud(false)

	// The first frame shows live silence.
	z.SetActiveTab(TabMeters)
	draw()

	if got := z.levelsLatches.Get(channel.Master).PeakHoldDB; got > analyzerDBFloor+1 {
		t.Fatalf("Levels meter stale after return: master PeakHoldDB=%v dB, want ~%v dB (live silence)", got, analyzerDBFloor)
	}
}

// It snaps UP too: real-time accuracy, not "silence on entry".
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

	if got := z.levelsLatches.Get(channel.Master).PeakHoldDB; got < -10 {
		t.Fatalf("Levels meter did not track live loud level after return: master PeakHoldDB=%v dB, want ~-3 dB", got)
	}
}

// While the tab stays visible a transient is still held; the reset fires only after the tab was hidden.
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

	// Still pinned well above the floor.
	if got := z.levelsLatches.Get(channel.Master).PeakHoldDB; got <= analyzerDBFloor+1 {
		t.Fatalf("peak-hold ballistics lost during continuous viewing: PeakHoldDB=%v dB, want held near -3 dB", got)
	}
}
