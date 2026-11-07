package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Real playback with rapid live edits must never false-panic, injected drift must always be caught, and pause /
// resume must leave the gate healthy. The gate is dormant under go test, so each test arms it through
// enableParityTimingForTest, which also restores it.

func loadStressCircuitForTest(t *testing.T, g *Game, rows, bpm int) {
	t.Helper()
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	buildSoakScene(t, g, rows, 8)
	g.drum.SetBPM(bpm)
}

// A real mutation through the gate: bumps parityGen and grants grace.
func cycleInstrumentForTest(g *Game, row int) {
	if g == nil || g.drum == nil || len(g.drum.Rows) == 0 {
		return
	}
	row %= len(g.drum.Rows)
	if row < 0 {
		row += len(g.drum.Rows)
	}
	g.drum.selRow = row
	g.drum.CycleInstrument()
}

// With a play func set the scheduler takes the direct-dispatch path, so only Stage A (seqfire) gets real samples
// here; B/C/D stay under MinSamples. The synthetic-feed test below covers them.
func TestParityTimingStressRealPlaybackNoFalsePositive(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	const rows = 8
	loadStressCircuitForTest(t, g, rows, 240)

	g.SetPlaying(true)
	advanceFrames(g, 5)

	for i := 0; i < 3000; i++ {
		advancePlaybackByAbs(g, 1)
		switch {
		case i%250 == 0:
			cycleInstrumentForTest(g, i%rows)
		case i%400 == 0:
			g.drum.toggleMute((i / 400) % rows)
		case i%600 == 0:
			g.drum.toggleSolo(0)
		}
		g.refreshDrumRow() // drives parityScan + parityTimingCheck
	}
}

// Healthy samples for all four stages interleaved with live edits; grace is off under go test, so every check is a
// real four-stage evaluation, never masked.
func TestParityTimingStressSyntheticFeedNoFalsePositive(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	const rows = 8
	loadStressCircuitForTest(t, g, rows, 240)
	g.SetPlaying(true)

	for i := 0; i < 4000; i++ {
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		g.schedMetrics.ObserveBridge(0.0005)
		// A constant audioNow would keep Stage C in warm-up and inert.
		aNow := float64(i) * 0.01
		g.schedMetrics.Observe(aNow, aNow+0.02)
		g.schedMetrics.ObserveEmitDrift(0.001)
		switch {
		case i%250 == 0:
			cycleInstrumentForTest(g, i%rows)
		case i%400 == 0:
			g.drum.toggleMute((i / 400) % rows)
		case i%600 == 0:
			g.drum.toggleSolo(0)
		}
		g.parityTimingCheck()
	}
}

// A systematically late stream breaches the mean once MinSamples is reached though no sample breaches AbsMax.
// Stage C, because emitdrift only warns.
func TestParityTimingStressCatchesInjectedDrift(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	g.schedMetrics.Observe(0.01, 0.01) // prime startAt (non-zero: 0 is the "unset" sentinel); dropped by warmup
	for i := 0; i < 200; i++ {
		aNow := 1.0 + float64(i)*0.001
		g.schedMetrics.Observe(aNow, aNow-0.001) // lag 1ms > Overdue.MeanMax 0.5ms, < AbsMax 20ms
	}
	g.schedMetrics.ObserveEmitDrift(0.05) // emission also drifted late -> confirms the overdue is fatal
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })
}

// Paused, the check no-ops; resuming drains a catch-up burst whose lateness must stay inside healthy Stage A
// bounds.
func TestParityTimingStressPauseResumeNoFalsePositive(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	const rows = 4
	loadStressCircuitForTest(t, g, rows, 240)

	g.SetPlaying(true)
	advanceFrames(g, 5)

	for i := 0; i < 800; i++ {
		advancePlaybackByAbs(g, 1)
		g.refreshDrumRow()
	}

	// Silent whatever wall-clock drift accrues while paused.
	g.SetPausedForTest(true)
	t.Cleanup(func() { g.SetPausedForTest(false) })
	for i := 0; i < 50; i++ {
		advancePlaybackByAbs(g, 1)
		g.refreshDrumRow()
	}

	// A live edit right at the seam, where a catch-up burst is most likely.
	g.SetPausedForTest(false)
	for i := 0; i < 800; i++ {
		advancePlaybackByAbs(g, 1)
		if i == 0 || i%150 == 0 {
			cycleInstrumentForTest(g, i%rows)
		}
		g.refreshDrumRow()
	}
}
