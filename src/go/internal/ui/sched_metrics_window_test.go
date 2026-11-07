package ui

import (
	"testing"
	"time"
)

// A window reset tied to the PERF_LOG cadence never reached MinSamples at normal tempos, silently disabling
// the statistical stages; the window is a frame-end rolling reset whatever PERF_LOG says.
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

	// About 20 Stage-C observations/s need more than 3.2 s to reach MinSamples=64.
	if minWindow := 5 * time.Second; schedMetricsWindow < minWindow {
		t.Fatalf("schedMetricsWindow=%v too short for MinSamples=64 at normal tempos (want >= %v)", schedMetricsWindow, minWindow)
	}
}
