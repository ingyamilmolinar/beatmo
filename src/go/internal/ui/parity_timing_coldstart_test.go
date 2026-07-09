package ui

import (
	"testing"
	"time"
)

// TestParityTimingColdStartSpikeClearedBySettleWindow is the TDD repro +
// regression test for the cold-start sibling of the C1 bug
// (parity_timing_liveedit_test.go). The gate's cold-start settle guard
// (parityTimingCheck, game_parity_timing.go:115) suppresses the CHECK for the
// first schedWarmupSec (0.5s) of playback, but Stage B (bridge) absMax is a
// session-monotonic running max (schedule_metrics.go) that never resets in
// production. A genuine cold-start bridge spike — audio-device warmup, first
// synth renders, GC, goroutine contention pushing one enqueue→dispatch
// queue-wait to ~83ms — is OBSERVED during the warmup window and latches into
// bridgeMax. The moment the settle guard lifts, the very next check reads the
// latched cold-start bridgeMax (0.083s > desktop Bridge.AbsMax 0.030s) and the
// absMax dropout branch (fires regardless of MinSamples) panics — a false
// positive on a plain "click Run in the startup demo" session.
//
// This is the exact same suppress-the-check-not-the-observation defect C1
// fixed for the grace/edit path via schedMetrics.ResetPeaks(); the cold-start
// settle path was left with the identical latent hole. The fix mirrors C1:
// while the settle guard is active, clear the peaks so a warmup spike cannot
// survive the window.
func TestParityTimingColdStartSpikeClearedBySettleWindow(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// Feed a healthy, past-MinSamples window for all four stages so the gate
	// evaluates a populated window (not a MinSamples no-op) once the settle
	// window lifts — same magnitudes/rationale as the C1 tests.
	for i := 0; i < 200; i++ {
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		aNow := float64(i) * 0.01
		g.schedMetrics.Observe(aNow, aNow+0.02) // healthy lead +20ms, never overdue
		g.schedMetrics.ObserveEmitDrift(0.001)
	}
	// A large, healthy bridge population so the single cold-start spike below
	// stays beyond p99 and negligible in mean/stddev — only the
	// session-monotonic bridgeMax (absMax dropout, fires regardless of
	// MinSamples) catches it, exactly as in production.
	for i := 0; i < 2000; i++ {
		g.schedMetrics.ObserveBridge(0.0005)
	}
	// The cold-start bridge spike observed DURING warmup: 0.083s, far beyond
	// desktop Bridge.AbsMax (0.030s). This is the value from the field report.
	g.schedMetrics.ObserveBridge(0.083)

	if snap := g.schedMetrics.Snapshot(); snap.BridgeMax < 0.083 {
		t.Fatalf("setup: expected latched BridgeMax>=0.083, got %v", snap.BridgeMax)
	}

	// Settle window ACTIVE: PlayStart is recent (< schedWarmupSec). Cold-start
	// spikes are expected here; the gate must not let them latch. The check is
	// suppressed — and (fix) must clear the session-monotonic peaks so the
	// spike cannot outlive the warmup window.
	g.SetPlayStartForTest(time.Now())
	g.parityTimingCheck() // suppressed by settle guard; must not panic

	// RED before fix: the settle guard suppressed the CHECK but never cleared
	// the OBSERVATION, so the cold-start spike is still latched.
	if snap := g.schedMetrics.Snapshot(); snap.BridgeMax != 0 {
		t.Fatalf("cold-start spike survived the settle window: BridgeMax=%v, want 0", snap.BridgeMax)
	}

	// Warmup window expires: PlayStart recedes past schedWarmupSec, so the next
	// check evaluates for real. Steady-state is healthy; it must NOT inherit
	// the cold-start spike cleared during warmup.
	g.SetPlayStartForTest(time.Now().Add(-time.Duration((schedWarmupSec + 0.1) * float64(time.Second))))
	g.parityTimingCheck() // before fix: panics bridge/absmax
}
