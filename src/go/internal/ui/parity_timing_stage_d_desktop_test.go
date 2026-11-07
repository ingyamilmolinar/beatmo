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

// A report whose generation moved between schedule and emission (a live edit) is dropped, not folded into the
// drift window.
func TestStageDDriftGenFilterDropsStaleReport(t *testing.T) {
	g := newTestGameForParity(t)
	g.SetPlaying(true)

	g.openTimingTrace(0, 5, 1.000, g.audioGen.Load(), g.parityGen.Load())

	// The edit lands between the trace opening and the report.
	g.audioGen.Add(1)

	feedEmissionReportForTest(t, g, 0, 5, 1.008)

	s := g.schedMetrics.Snapshot()
	if s.EmitCount != 0 {
		t.Fatalf("EmitCount=%d want 0 (stale report must be dropped by gen filter)", s.EmitCount)
	}
}

// DrainEmitReports needs a device position; closeTimingTrace is the seam it calls after mapping to wall time.
func feedEmissionReportForTest(t *testing.T, g *Game, row, abs int, mappedWallSec float64) {
	t.Helper()
	g.closeTimingTrace(row, abs, mappedWallSec)
}
