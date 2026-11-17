package ui

import (
	"math"
	"testing"
)

func TestStageDDriftFedFromEmission(t *testing.T) {
	g := newTestGameForParity(t)
	g.SetPlaying(true)

	// Open a trace: ideal emission at t=1.000s.
	g.openTimingTrace(0, 5, 1.000, g.audioGen.Load(), g.parityGen.Load())

	// Simulate an emission report whose mapped wall-time is 1.008s → +8ms drift.
	feedEmissionReportForTest(t, g, 0, 5 /*mappedWallSec=*/, 1.008)

	s := g.schedMetrics.Snapshot()
	if s.EmitCount != 1 {
		t.Fatalf("EmitCount=%d want 1", s.EmitCount)
	}
	if math.Abs(s.EmitAbsMax-0.008) > 1e-6 {
		t.Errorf("EmitAbsMax=%.6f want 0.008", s.EmitAbsMax)
	}
}

// TestStageDDriftGenFilterDropsStaleReport proves live-edit safety for Stage
// D: an emission report that lands for a trace whose generation no longer
// matches the current audioGen/parityGen (i.e. a live edit happened between
// schedule and emission) must be dropped, not folded into the drift window.
func TestStageDDriftGenFilterDropsStaleReport(t *testing.T) {
	g := newTestGameForParity(t)
	g.SetPlaying(true)

	g.openTimingTrace(0, 5, 1.000, g.audioGen.Load(), g.parityGen.Load())

	// Simulate a live structural edit bumping the generation after the trace
	// was opened but before the emission report arrives.
	g.audioGen.Add(1)

	feedEmissionReportForTest(t, g, 0, 5, 1.008)

	s := g.schedMetrics.Snapshot()
	if s.EmitCount != 0 {
		t.Fatalf("EmitCount=%d want 0 (stale report must be dropped by gen filter)", s.EmitCount)
	}
}

// feedEmissionReportForTest bridges the desktop-only audio.DrainEmitReports
// (which needs a device pos, not a wall time) — closeTimingTrace is the seam
// drainEmissionReports calls after mapping devPos→wall, so tests drive
// Stage-D directly through it.
func feedEmissionReportForTest(t *testing.T, g *Game, row, abs int, mappedWallSec float64) {
	t.Helper()
	g.closeTimingTrace(row, abs, mappedWallSec)
}
