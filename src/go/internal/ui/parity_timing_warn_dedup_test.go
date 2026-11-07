package ui

import (
	"math"
	"testing"
	"time"
)

// The check runs every frame; a latched warn-only breach would re-dump its WARN block on every scan. One dump per
// (stage, stat) per metrics window.
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

// Clearing only the peaks left warm-up spikes in the percentile reservoirs, polluting mean/p99/stddev for the
// rest of the window.
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
	// An empty window: count 0, absMax 0, NaN percentile.
	if snap.EmitCount != 0 || snap.EmitAbsMax != 0 || !math.IsNaN(snap.EmitP99) {
		t.Fatalf("warmup spikes survived the settle guard: EmitCount=%d EmitP99=%v EmitAbsMax=%v (want 0 / NaN / 0)", snap.EmitCount, snap.EmitP99, snap.EmitAbsMax)
	}
}
