package ui

import (
	"math"
	"testing"
)

// Signed samples feed the bias mean, |drift| the tail metrics. Unlike Stage C's Observe, this does not gate on
// the warm-up, so none is needed.
func TestScheduleMetricsEmitDrift(t *testing.T) {
	m := &scheduleMetrics{}
	for _, d := range []float64{0.001, -0.002, 0.003, -0.001} {
		m.ObserveEmitDrift(d)
	}
	s := m.Snapshot()
	if s.EmitCount != 4 {
		t.Fatalf("EmitCount=%d want 4", s.EmitCount)
	}
	if math.Abs(s.EmitAbsMax-0.003) > 1e-9 {
		t.Errorf("EmitAbsMax=%.6f want 0.003", s.EmitAbsMax)
	}
	// signed mean = (0.001-0.002+0.003-0.001)/4 = 0.00025
	if math.Abs(s.EmitAvg-0.00025) > 1e-9 {
		t.Errorf("EmitAvg=%.6f want 0.00025", s.EmitAvg)
	}
	if s.EmitStdDev <= 0 || math.IsNaN(s.EmitStdDev) {
		t.Errorf("EmitStdDev should be defined and > 0, got %v", s.EmitStdDev)
	}
	if math.IsNaN(s.EmitP99) {
		t.Errorf("EmitP99 should be defined, got NaN")
	}
	m.Reset()
	rs := m.Snapshot()
	if rs.EmitCount != 0 {
		t.Error("Reset did not clear emit stage")
	}
	if !math.IsNaN(rs.EmitAvg) || !math.IsNaN(rs.EmitStdDev) || !math.IsNaN(rs.EmitP99) {
		t.Errorf("Reset should leave Emit derived fields as NaN, got Avg=%v StdDev=%v P99=%v", rs.EmitAvg, rs.EmitStdDev, rs.EmitP99)
	}
}
