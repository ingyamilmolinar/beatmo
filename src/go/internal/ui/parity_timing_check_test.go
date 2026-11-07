package ui

import "testing"

// A dropout-class overdue PLUS a late emission: Stage C panics only on confirmed audible lateness; an on-time
// emission makes it a WARN.
func feedOverdueDropoutForTest(g *Game) {
	// Non-zero: startAt==0 is the "unset" sentinel, so a 0.0 prime would be re-primed and dropped.
	g.schedMetrics.Observe(0.01, 0.01)    // sets startAt=0.01; dropped by the 0.5s warmup
	g.schedMetrics.Observe(1.0, 0.95)     // lag = 0.05s > Overdue.AbsMax (0.020s)
	g.schedMetrics.ObserveEmitDrift(0.05) // emission drifted 50ms late -> confirms audible lateness
}

// A UI action stalls the audioLoop so a beat dispatches overdue, yet the device emitted it on time.
func feedOverdueSchedulingBlipForTest(g *Game) {
	g.schedMetrics.Observe(0.01, 0.01)
	g.schedMetrics.Observe(1.0, 0.95)      // lag 0.05s > Overdue.AbsMax
	g.schedMetrics.ObserveEmitDrift(0.005) // emission on-time (5ms) — no audible lateness
}

func TestParityTimingCheckPanicsOnOverdueDropout(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	feedOverdueDropoutForTest(g)

	assertPanics(t, "overdue", func() { g.parityTimingCheck() })
}

// The Stage-C↔Stage-D cross-check downgrades it to a WARN.
func TestParityTimingOverdueWithOnTimeEmitIsWarnOnly(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	feedOverdueSchedulingBlipForTest(g)

	// Must not panic — overdue is real but the emission was on-time.
	g.parityTimingCheck()
}

// Seqfire, bridge and emitdrift measure scheduling proxies the lookahead absorbs (wake-up jitter, queue wait,
// device emission jitter), so they only warn; Stage C stays the fatal gate. Per-beat ideal baseNow keeps Stage A
// lateness from reaching the audio.
func TestParityTimingBridgeEmitDriftAreWarnOnly(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// Each well past its desktop AbsMax.
	g.schedMetrics.ObserveSeqFireLate(0.090)
	g.schedMetrics.ObserveBridge(0.090)
	g.schedMetrics.ObserveEmitDrift(0.090)

	// Must not panic — a WarnOnly breach logs a WARN dump and falls through.
	g.parityTimingCheck()
}

func TestParityTimingCheckNoPanicUnderBounds(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// 200 healthy samples across stages, all well under bounds.
	for i := 0; i < 200; i++ {
		g.schedMetrics.ObserveEmitDrift(0.001)
		g.schedMetrics.ObserveBridge(0.0005)
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		g.schedMetrics.Observe(1.0, 1.02) // healthy lead, no lag
	}
	// Must not panic.
	g.parityTimingCheck()
}

func TestParityTimingCheckSuppressedWhenStopped(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(false) // stopped

	feedOverdueDropoutForTest(g) // would breach (fatal) if playing
	g.parityTimingCheck()        // no panic
}
