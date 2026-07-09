package ui

import "testing"

// TestScheduleMetricsLagStatsNormalizedOverAllDispatches pins the fix for the
// 300 BPM overdue/mean false-fatal (2026-07-13): AvgLag/LagP99 were computed
// over ONLY the overdue samples (a conditional "mean lateness of the late
// beats"), so a SINGLE 4ms blip in a 65-dispatch window reported
// AvgLag=LagP99=4ms and breached the 0.5ms MeanMax bound written for
// sustained drift. Lag statistics must be normalized over ALL dispatches in
// the window — on-time dispatches contribute lag 0 — so an isolated blip
// stays in the absmax (dropout) statistic where the emit cross-check governs
// it, and the mean/p99 bounds keep their sustained-lateness meaning.
func TestScheduleMetricsLagStatsNormalizedOverAllDispatches(t *testing.T) {
	var m scheduleMetrics
	// Prime past the warmup window (startAt sentinel is audioNow==0; use a
	// non-zero prime, then feed from beyond startAt+schedWarmupSec).
	m.Observe(1.0, 1.01)

	// 64 healthy dispatches (5ms lead) and ONE 4ms-overdue blip.
	now := 2.0
	for i := 0; i < 64; i++ {
		m.Observe(now, now+0.005)
		now += 0.01
	}
	m.Observe(now, now-0.004)

	s := m.Snapshot()
	if s.Count != 65 {
		t.Fatalf("count want=65 got=%d", s.Count)
	}
	if s.Overdue != 1 {
		t.Fatalf("overdue want=1 got=%d", s.Overdue)
	}
	// Mean over all 65 dispatches: 4ms/65 ≈ 0.0615ms — far under the 0.5ms
	// sustained-drift bound.
	if want := 0.004 / 65.0; !approxEq(s.AvgLag, want, 1e-9) {
		t.Errorf("AvgLag want=%v (mean over all dispatches) got=%v", want, s.AvgLag)
	}
	// 99th-percentile dispatch was on-time: 1 lag in 65 is a <2%... a 1.5%
	// overdue fraction still exceeds 1%, so p99 lands inside the lag tail —
	// use p90 for the zero case and p99 for the in-tail case.
	if s.LagP90 != 0 {
		t.Errorf("LagP90 want=0 (90th-percentile dispatch on-time) got=%v", s.LagP90)
	}
	// Dropout statistic keeps the single-sample semantics.
	if !approxEq(s.MaxLag, 0.004, 1e-9) {
		t.Errorf("MaxLag want=4ms got=%v", s.MaxLag)
	}

	// Sustained lateness must still trip the mean: every dispatch 1ms late.
	var m2 scheduleMetrics
	m2.Observe(1.0, 1.01)
	now = 2.0
	for i := 0; i < 100; i++ {
		m2.Observe(now, now-0.001)
		now += 0.01
	}
	s2 := m2.Snapshot()
	if !approxEq(s2.AvgLag, 0.001, 1e-9) {
		t.Errorf("sustained AvgLag want=1ms got=%v", s2.AvgLag)
	}
	if !approxEq(s2.LagP99, 0.001, 1e-9) {
		t.Errorf("sustained LagP99 want=1ms got=%v", s2.LagP99)
	}
}
