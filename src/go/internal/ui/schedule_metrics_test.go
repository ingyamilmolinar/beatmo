package ui

import (
	"math"
	"testing"
)

// approxEq is a relative-tolerance equality check.
func approxEq(a, b, tol float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) == math.IsNaN(b)
	}
	d := math.Abs(a - b)
	return d <= tol || d <= tol*math.Max(math.Abs(a), math.Abs(b))
}

// TestScheduleMetricsThreeStages verifies that Observe / ObserveBridge /
// ObserveSeqFireLate each populate their own stage independently and
// expose count, percentiles, and stddev in the snapshot.
func TestScheduleMetricsThreeStages(t *testing.T) {
	var m scheduleMetrics

	// Drive past the 0.5s warmup window with a fake startAt of 0.
	// Observe with audioNow > schedWarmupSec.
	const baseNow = 1.0
	const baseWhen = 1.05 // lead = 50ms

	// 100 events with lead values jittering around 50ms ± 10ms in a
	// triangle wave so percentiles and stddev are well-defined.
	for i := 0; i < 100; i++ {
		offset := (float64(i%20) - 10) * 0.001 // -10ms..+10ms
		m.Observe(baseNow+float64(i)*0.01, baseWhen+float64(i)*0.01+offset)
	}

	// Bridge: 50 samples between 1ms and 5ms.
	for i := 0; i < 50; i++ {
		m.ObserveBridge(0.001 + float64(i)*0.0001) // 1ms..6ms
	}

	// SeqFireLate: 50 samples between 0.5ms and 2.5ms.
	for i := 0; i < 50; i++ {
		m.ObserveSeqFireLate(0.0005 + float64(i)*0.00005) // 0.5ms..3ms
	}

	s := m.Snapshot()

	// Stage C: lead.
	if s.Count == 0 {
		t.Fatalf("Stage C: count == 0, want > 0")
	}
	if math.IsNaN(s.LeadP99) || math.IsNaN(s.LeadStdDev) {
		t.Fatalf("Stage C: percentile/stddev should be defined: P99=%v stddev=%v", s.LeadP99, s.LeadStdDev)
	}
	if s.LeadStdDev <= 0 {
		t.Fatalf("Stage C: stddev should be > 0 for jittered samples, got %v", s.LeadStdDev)
	}
	// Triangle wave ±10ms around mean → stddev ≈ 6ms.
	if !approxEq(s.LeadStdDev, 0.006, 0.002) {
		t.Errorf("Stage C: LeadStdDev expected ~6ms, got %.6f", s.LeadStdDev)
	}

	// Stage B: bridge.
	if s.BridgeCount != 50 {
		t.Fatalf("Stage B: bridgeCount want=50 got=%d", s.BridgeCount)
	}
	if !(s.BridgeAvg > 0.002 && s.BridgeAvg < 0.005) {
		t.Errorf("Stage B: bridgeAvg outside [2ms,5ms]: %v", s.BridgeAvg)
	}
	if s.BridgeP99 < s.BridgeP50 {
		t.Errorf("Stage B: P99 < P50 violates monotonicity: P50=%v P99=%v", s.BridgeP50, s.BridgeP99)
	}
	if math.IsNaN(s.BridgeStdDev) || s.BridgeStdDev <= 0 {
		t.Errorf("Stage B: stddev should be > 0, got %v", s.BridgeStdDev)
	}

	// Stage A: seq-fire-late.
	if s.SeqFireCount != 50 {
		t.Fatalf("Stage A: seqFireCount want=50 got=%d", s.SeqFireCount)
	}
	if !(s.SeqFireAvg > 0.001 && s.SeqFireAvg < 0.0025) {
		t.Errorf("Stage A: seqFireAvg outside [1ms,2.5ms]: %v", s.SeqFireAvg)
	}
	if s.SeqFireP99 < s.SeqFireP50 {
		t.Errorf("Stage A: P99 < P50: P50=%v P99=%v", s.SeqFireP50, s.SeqFireP99)
	}

	// E2E P99 should be ~ sum of stage P99s (no overdue lag here).
	want := s.SeqFireP99 + s.BridgeP99
	if !approxEq(s.E2EP99, want, 0.0001) {
		t.Errorf("E2EP99 want≈%v got %v", want, s.E2EP99)
	}
}

// TestScheduleMetricsOverdueAccumulatesLag confirms negative lead values
// flow into lagP90/lagP99 and not into lead stddev as positive jitter.
func TestScheduleMetricsOverdueAccumulatesLag(t *testing.T) {
	var m scheduleMetrics

	// Past warmup window.
	now := 1.0
	// Half on-time (lead = 5ms), half overdue (lead = -3ms = lag 3ms).
	for i := 0; i < 50; i++ {
		m.Observe(now, now+0.005)
		now += 0.01
	}
	for i := 0; i < 50; i++ {
		m.Observe(now, now-0.003)
		now += 0.01
	}

	s := m.Snapshot()
	if s.Overdue != 50 {
		t.Errorf("overdue want=50 got=%d", s.Overdue)
	}
	if !approxEq(s.MaxLag, 0.003, 1e-6) {
		t.Errorf("maxLag want=3ms got=%v", s.MaxLag)
	}
	if math.IsNaN(s.LagP99) || s.LagP99 <= 0 {
		t.Errorf("LagP99 should be positive, got %v", s.LagP99)
	}
}

// TestScheduleMetricsResetClearsAllStages ensures Reset wipes all three
// stages so the next playback session starts fresh.
func TestScheduleMetricsResetClearsAllStages(t *testing.T) {
	var m scheduleMetrics
	m.Observe(1.0, 1.05)
	m.ObserveBridge(0.002)
	m.ObserveSeqFireLate(0.001)
	m.Reset()
	s := m.Snapshot()
	if s.Count != 0 || s.BridgeCount != 0 || s.SeqFireCount != 0 {
		t.Fatalf("Reset failed: count=%d bridge=%d seqFire=%d", s.Count, s.BridgeCount, s.SeqFireCount)
	}
	if !math.IsNaN(s.LeadP99) || !math.IsNaN(s.BridgeP99) || !math.IsNaN(s.SeqFireP99) {
		t.Errorf("after reset percentiles should be NaN")
	}
}

// TestStdDevNumericalStability guards against negative variance from
// floating-point cancellation on near-constant samples.
func TestStdDevNumericalStability(t *testing.T) {
	if v := stddev(100*0.05, 100*0.05*0.05, 100); v != 0 {
		t.Errorf("constant-sample stddev want 0, got %v", v)
	}
	if v := stddev(0.05, 0.05*0.05, 1); !math.IsNaN(v) {
		t.Errorf("n=1 stddev should be NaN, got %v", v)
	}
}
