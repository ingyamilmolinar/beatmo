package ui

import "testing"

// feedOverdueDropoutForTest primes Stage C's warmup and injects a single
// dropout-class overdue reading (lag > desktop Overdue.AbsMax 0.020s), PLUS a
// dropout-class Stage-D emission drift (> OverdueEmitFatalSec 0.020s) so the
// overdue is a CONFIRMED audible-lateness event (the emission actually came out
// late). Stage C only panics when the emission also drifted — a bare scheduling
// blip that emitted on-time is a WARN (see feedOverdueSchedulingBlipForTest).
func feedOverdueDropoutForTest(g *Game) {
	// Prime startAt with a small NON-zero audioNow: startAt==0 doubles as the
	// "unset" sentinel in scheduleMetrics.Observe, so a 0.0 prime would be
	// re-primed (and dropped) by the next call.
	g.schedMetrics.Observe(0.01, 0.01)    // sets startAt=0.01; dropped by the 0.5s warmup
	g.schedMetrics.Observe(1.0, 0.95)     // lag = 0.05s > Overdue.AbsMax (0.020s)
	g.schedMetrics.ObserveEmitDrift(0.05) // emission drifted 50ms late -> confirms audible lateness
}

// feedOverdueSchedulingBlipForTest injects the same dropout-class overdue but
// with an ON-TIME emission (drift well under OverdueEmitFatalSec) — the
// solo/un-solo crash scenario: a UI action briefly stalls the audioLoop so a
// beat is dispatched "overdue", but the audio device still emitted it on time.
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

// TestParityTimingOverdueWithOnTimeEmitIsWarnOnly is the regression for the
// solo/un-solo crash: an overdue scheduling blip whose emission still came out
// on-time must NOT panic (it downgrades to a WARN via the Stage-C↔Stage-D
// cross-check). Guards runtime_profile.go's OverdueEmitFatalSec.
func TestParityTimingOverdueWithOnTimeEmitIsWarnOnly(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	feedOverdueSchedulingBlipForTest(g)

	// Must not panic — overdue is real but the emission was on-time.
	g.parityTimingCheck()
}

// TestParityTimingBridgeEmitDriftAreWarnOnly pins the calibration decision: a
// dropout-class seqfire, bridge AND emitdrift reading — each far beyond its
// AbsMax bound — must NOT panic, because those stages measure internal
// scheduling proxies (sequencer wakeup jitter; dispatch queue-wait;
// device-clock emission jitter) that the audio lookahead absorbs. Stage C
// (overdue) remains the fatal audible-lateness gate. Regression guard for
// runtime_profile.go's SeqFire/Bridge/EmitDrift WarnOnly: true.
//
// SeqFire joined the WarnOnly set 2026-07-13: a 300 BPM bench crashed on
// seqfire/p99 from a single ~19ms Go-scheduler wakeup delay (execution trace:
// selectnbsend on hlCh) while Stage C stayed at zero — per-beat ideal baseNow
// means Stage A lateness never propagates into audio lateness.
func TestParityTimingBridgeEmitDriftAreWarnOnly(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// All well past their desktop AbsMax (seqfire 0.030s, bridge 0.030s,
	// emitdrift 0.040s).
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
