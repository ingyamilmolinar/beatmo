package ui

import (
	"testing"
	"time"
)

// The settle guard suppresses the CHECK for the first schedWarmupSec, but the peaks are session-monotonic: a
// cold-start spike observed in warm-up would latch and panic the first real check. While the guard is active the
// peaks must be cleared.
func TestParityTimingColdStartSpikeClearedBySettleWindow(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// A populated, past-MinSamples window, so the check after warm-up is a real evaluation.
	for i := 0; i < 200; i++ {
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		aNow := float64(i) * 0.01
		g.schedMetrics.Observe(aNow, aNow+0.02) // healthy lead +20ms, never overdue
		g.schedMetrics.ObserveEmitDrift(0.001)
	}
	// Large enough that the single spike stays out of p99, mean and stddev: only the monotonic max sees it.
	for i := 0; i < 2000; i++ {
		g.schedMetrics.ObserveBridge(0.0005)
	}
	// Observed DURING warm-up, far beyond desktop Bridge.AbsMax.
	g.schedMetrics.ObserveBridge(0.083)

	if snap := g.schedMetrics.Snapshot(); snap.BridgeMax < 0.083 {
		t.Fatalf("setup: expected latched BridgeMax>=0.083, got %v", snap.BridgeMax)
	}

	// Settle window active: the check is suppressed and the peaks must be cleared.
	g.SetPlayStartForTest(time.Now())
	g.parityTimingCheck() // suppressed by settle guard; must not panic

	if snap := g.schedMetrics.Snapshot(); snap.BridgeMax != 0 {
		t.Fatalf("cold-start spike survived the settle window: BridgeMax=%v, want 0", snap.BridgeMax)
	}

	// Past warm-up the check runs for real and must not inherit the cleared spike.
	g.SetPlayStartForTest(time.Now().Add(-time.Duration((schedWarmupSec + 0.1) * float64(time.Second))))
	g.parityTimingCheck() // before fix: panics bridge/absmax
}
