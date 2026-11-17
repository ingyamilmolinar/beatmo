//go:build test

package ui

import (
	"runtime"
	"testing"
	"time"
)

// TestForceGCPacingRunsRuntimeGC asserts that heapProbeTick triggers a real
// runtime.GC() when ForceGCInterval is positive and at least the interval
// of wall-clock time has elapsed since the last forced GC. Catches the
// WASM GC-starvation regression (gc=0 across 5 min of synth-tab + param-
// drag playback) demonstrated on 2026-05-17 (Go heap grew 26.7 MB →
// 493 MB with gcCount=0).
func TestForceGCPacingRunsRuntimeGC(t *testing.T) {
	withDefaultAudio(t)

	prev := RuntimeProf().ForceGCInterval
	t.Cleanup(func() { RuntimeProf().ForceGCInterval = prev })
	RuntimeProf().ForceGCInterval = 50 * time.Millisecond
	ResetForcedGCRunsForTest()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// First tick establishes forcedGCLastFired; never fires GC.
	g.frame = 1
	g.heapProbeTick()

	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	// Three sleep+tick cycles, each 60ms apart — first interval-elapsed
	// tick fires; we expect ~2-3 forced GCs in 180ms.
	for i := 0; i < 3; i++ {
		time.Sleep(60 * time.Millisecond)
		g.frame = int64(i + 2)
		g.heapProbeTick()
	}
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if runs := ForcedGCRunsForTest(); runs < 2 {
		t.Fatalf("expected >= 2 forced-GC trigger runs after 3 ticks at 60ms apart with 50ms interval, got %d", runs)
	}
	if delta := after.NumGC - before.NumGC; delta < 2 {
		t.Fatalf("expected >= 2 GC runs (NumGC delta) corresponding to forced triggers, got delta=%d (forcedGCRuns=%d)",
			delta, ForcedGCRunsForTest())
	}
}

// TestForceGCPacingDisabledByZeroInterval asserts that the forced-GC
// path is inactive when ForceGCInterval=0 (desktop default).
// Spontaneous GC may still happen under allocation pressure, but the
// intentional path should fire zero times via forcedGCRuns.
func TestForceGCPacingDisabledByZeroInterval(t *testing.T) {
	withDefaultAudio(t)

	prev := RuntimeProf().ForceGCInterval
	t.Cleanup(func() { RuntimeProf().ForceGCInterval = prev })
	RuntimeProf().ForceGCInterval = 0
	ResetForcedGCRunsForTest()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	for i := 0; i < 5; i++ {
		time.Sleep(20 * time.Millisecond)
		g.frame = int64(i + 1)
		g.heapProbeTick()
	}

	if runs := ForcedGCRunsForTest(); runs != 0 {
		t.Fatalf("expected forced-GC path inactive at interval=0, got forcedGCRuns=%d", runs)
	}
}
