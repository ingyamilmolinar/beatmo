package ui

import "testing"

// Lag statistics are normalised over ALL dispatches (on-time ones contribute 0), so one isolated blip stays in
// the absmax statistic and the mean/p99 bounds keep their sustained-lateness meaning.
func TestScheduleMetricsLagStatsNormalizedOverAllDispatches(t *testing.T) {
	var m scheduleMetrics
	// A non-zero prime (0 is the startAt sentinel), then samples past the warm-up.
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
	// 4 ms over 65 dispatches, far under the sustained-drift bound.
	if want := 0.004 / 65.0; !approxEq(s.AvgLag, want, 1e-9) {
		t.Errorf("AvgLag want=%v (mean over all dispatches) got=%v", want, s.AvgLag)
	}
	// One late dispatch in 65 is above 1 %, so p99 lands in the lag tail; p90 checks the zero case.
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
