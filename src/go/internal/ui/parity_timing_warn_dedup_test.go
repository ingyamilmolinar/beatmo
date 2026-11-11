package ui

import (
	"math"
	"testing"
	"time"
)

// TestTimingWarnOnlyDumpDedupedPerWindow pins the WarnOnly dump rate limit
// (2026-07-13): parityTimingCheck runs every frame, and a latched WarnOnly
// breach re-dumped its full WARN block on every scan until the metrics window
// reset (~60 dumps/s; 4788 dumps in a 90s bench). emitTimingFatal dedups
// non-fatal dumps to one per (stage, stat) per metrics window; the window
// reset refreshes the budget.
func TestTimingWarnOnlyDumpDedupedPerWindow(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// Latch a WarnOnly bridge dropout and scan several frames' worth.
	g.schedMetrics.ObserveBridge(0.090)
	for i := 0; i < 5; i++ {
		g.parityTimingCheck()
	}
	if got := len(g.timingWarnedThisWindow); got != 1 {
		t.Fatalf("warn dedup: want 1 recorded (stage,stat) after 5 scans of one latched breach, got %d: %v", got, g.timingWarnedThisWindow)
	}
	if _, ok := g.timingWarnedThisWindow["bridge/absmax"]; !ok {
		t.Fatalf("warn dedup key want bridge/absmax, got %v", g.timingWarnedThisWindow)
	}

	// Window reset refreshes the budget (and wipes the latched metrics).
	base := time.Now()
	g.maybeResetSchedMetricsWindow(base)                                       // arm
	g.maybeResetSchedMetricsWindow(base.Add(schedMetricsWindow + time.Second)) // reset
	if got := len(g.timingWarnedThisWindow); got != 0 {
		t.Fatalf("window reset must clear the warn budget, got %d entries", got)
	}
}

// TestSettleGuardClearsReservoirsNotJustPeaks pins the full-Reset settle
// guard (2026-07-13): clearing only the peak fields left warmup spikes in the
// percentile reservoirs, where they polluted mean/p99/stddev for the rest of
// the metrics window (field signature: emit.p99=83ms while emit.absMax=11.5ms,
// plus ~10s of stale WarnOnly dumps after every play start).
func TestSettleGuardClearsReservoirsNotJustPeaks(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// Cold-start emission spikes observed during warmup.
	for i := 0; i < 10; i++ {
		g.schedMetrics.ObserveEmitDrift(0.085)
	}
	if snap := g.schedMetrics.Snapshot(); snap.EmitCount == 0 {
		t.Fatal("setup: expected warmup emit observations")
	}

	// Settle window active → suppressed scan must wipe the whole window.
	g.SetPlayStartForTest(time.Now())
	g.parityTimingCheck()

	snap := g.schedMetrics.Snapshot()
	// A fully-reset window reports EmitCount 0, EmitAbsMax 0 and a NaN
	// percentile (empty reservoir) — NOT the warmup spike values.
	if snap.EmitCount != 0 || snap.EmitAbsMax != 0 || !math.IsNaN(snap.EmitP99) {
		t.Fatalf("warmup spikes survived the settle guard: EmitCount=%d EmitP99=%v EmitAbsMax=%v (want 0 / NaN / 0)", snap.EmitCount, snap.EmitP99, snap.EmitAbsMax)
	}
}
