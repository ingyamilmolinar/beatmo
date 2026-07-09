package ui

import (
	"testing"
	"time"
)

// TestSchedMetricsWindowIndependentOfPerfLog pins the decoupling of the
// timing-gate metrics window from the PERF_LOG cadence (2026-07-13). The old
// reset lived inside the PERF_LOG block in game_update.go, so under run-debug
// (PERF_LOG=1) the window reset every 2s and never reached MinSamples=64 at
// normal tempos — the statistical stages of the timing gate were silently
// dead and only single-sample absmax checks were live. The window is now a
// frame-end rolling reset of schedMetricsWindow, unconditional on PERF_LOG.
func TestSchedMetricsWindowIndependentOfPerfLog(t *testing.T) {
	g := &Game{}
	g.schedMetrics.ObserveBridge(0.001)
	g.schedMetrics.ObserveBridge(0.002)
	if got := g.schedMetrics.Snapshot().BridgeCount; got != 2 {
		t.Fatalf("setup: BridgeCount want=2 got=%d", got)
	}

	base := time.Now()
	// First call arms the deadline; must not reset.
	g.maybeResetSchedMetricsWindow(base)
	if got := g.schedMetrics.Snapshot().BridgeCount; got != 2 {
		t.Fatalf("arming call reset the window: BridgeCount want=2 got=%d", got)
	}
	// Inside the window: no reset.
	g.maybeResetSchedMetricsWindow(base.Add(schedMetricsWindow / 2))
	if got := g.schedMetrics.Snapshot().BridgeCount; got != 2 {
		t.Fatalf("mid-window call reset early: BridgeCount want=2 got=%d", got)
	}
	// Past the deadline: reset and re-arm.
	g.maybeResetSchedMetricsWindow(base.Add(schedMetricsWindow + time.Second))
	if got := g.schedMetrics.Snapshot().BridgeCount; got != 0 {
		t.Fatalf("deadline passed but window not reset: BridgeCount want=0 got=%d", got)
	}

	// The window must be long enough for the statistical stages to arm at
	// normal tempos: ~20 Stage-C observations/s needs >3.2s to reach
	// MinSamples=64. Guards against re-tightening toward the old 2s cadence.
	if minWindow := 5 * time.Second; schedMetricsWindow < minWindow {
		t.Fatalf("schedMetricsWindow=%v too short for MinSamples=64 at normal tempos (want >= %v)", schedMetricsWindow, minWindow)
	}
}
