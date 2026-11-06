package fingerprint

import (
	"math"
	"testing"
)

func TestStatsOfLinearRamp(t *testing.T) {
	// 0..1 over 1 second at hop 0.1s => slope 1.0/sec, monotonic, median 0.5.
	tr := []float64{0, .1, .2, .3, .4, .5, .6, .7, .8, .9, 1}
	s := StatsOf(tr, 0.1)
	if math.Abs(s.Slope-1.0) > 1e-9 {
		t.Errorf("slope: got %v want 1.0", s.Slope)
	}
	if math.Abs(s.Median-0.5) > 1e-9 {
		t.Errorf("median: got %v want 0.5", s.Median)
	}
	if math.Abs(s.MonotonicFrac-1.0) > 1e-9 {
		t.Errorf("monotonic fraction: got %v want 1.0", s.MonotonicFrac)
	}
	if math.Abs(s.Range-1.0) > 1e-9 {
		t.Errorf("range: got %v want 1.0", s.Range)
	}
	if math.Abs(s.Curvature) > 1e-6 {
		t.Errorf("a straight ramp must have ~zero curvature; got %v", s.Curvature)
	}
	if s.N != len(tr) {
		t.Errorf("N: got %d want %d", s.N, len(tr))
	}
}

func TestStatsOfEmptyTrackIsZeroNotNaN(t *testing.T) {
	s := StatsOf(nil, 0.1)
	if math.IsNaN(s.Mean) || math.IsNaN(s.Slope) || math.IsNaN(s.Std) || math.IsNaN(s.Curvature) {
		t.Fatal("empty track must yield zeros, never NaN")
	}
	if s.N != 0 {
		t.Errorf("N: got %d want 0 (so a reader can tell zeros from measurements)", s.N)
	}
}

func TestStatsOfSingleSampleIsFinite(t *testing.T) {
	s := StatsOf([]float64{0.5}, 0.1)
	if math.IsNaN(s.Slope) || math.IsNaN(s.Curvature) || math.IsNaN(s.Std) {
		t.Fatal("a one-sample track must not produce NaN")
	}
	if s.Mean != 0.5 || s.Median != 0.5 {
		t.Errorf("mean/median: got %v/%v want 0.5/0.5", s.Mean, s.Median)
	}
}

// TestCurvatureSeparatesRampFromBloom is the shape half of the magnitude/shape
// split: two tracks with the SAME endpoints and a similar overall slope, one
// linear and one accelerating, must be distinguishable. A std- or range-based
// statistic cannot tell them apart.
func TestCurvatureSeparatesRampFromBloom(t *testing.T) {
	n := 21
	ramp := make([]float64, n)
	bloom := make([]float64, n)
	for i := range ramp {
		x := float64(i) / float64(n-1)
		ramp[i] = x
		bloom[i] = x * x // same endpoints, accelerating
	}
	rs := StatsOf(ramp, 0.05)
	bs := StatsOf(bloom, 0.05)

	if math.Abs(rs.Curvature) > 1e-6 {
		t.Errorf("linear ramp curvature: got %v want ~0", rs.Curvature)
	}
	if bs.Curvature <= 1e-6 {
		t.Errorf("accelerating bloom must show positive curvature; got %v", bs.Curvature)
	}
	if rs.Range != bs.Range {
		t.Fatalf("fixture broken: the two tracks must share a range (%v vs %v)", rs.Range, bs.Range)
	}
}

// TestMonotonicFracSeparatesGestureFromChaos is the discriminator for failure
// mode 2 at its cheapest: a slow shaped sweep and fast chaos can share a mean
// and a standard deviation while being completely different gestures.
func TestMonotonicFracSeparatesGestureFromChaos(t *testing.T) {
	n := 64
	gesture := make([]float64, n)
	chaos := make([]float64, n)
	state := uint32(7)
	for i := 0; i < n; i++ {
		gesture[i] = float64(i) / float64(n-1)
		state = state*1664525 + 1013904223
		chaos[i] = float64(state%1000) / 999.0
	}
	g := StatsOf(gesture, 0.01)
	c := StatsOf(chaos, 0.01)
	if g.MonotonicFrac <= c.MonotonicFrac {
		t.Fatalf("a shaped sweep (%v) must be more monotonic than chaos (%v)",
			g.MonotonicFrac, c.MonotonicFrac)
	}
}

func TestMonotonicFracOfFlatTrackIsZero(t *testing.T) {
	if got := StatsOf([]float64{1, 1, 1, 1}, 0.1).MonotonicFrac; got != 0 {
		t.Errorf("a flat track carries no gesture; got MonotonicFrac=%v want 0", got)
	}
}

func TestStatsSlopeIsPerSecondNotPerFrame(t *testing.T) {
	tr := []float64{0, 1, 2, 3, 4}
	fine := StatsOf(tr, 0.01)  // 1 unit per 0.01s = 100/sec
	coarse := StatsOf(tr, 0.1) // 1 unit per 0.1s  = 10/sec
	if math.Abs(fine.Slope-100) > 1e-6 {
		t.Errorf("fine slope: got %v want 100/sec", fine.Slope)
	}
	if math.Abs(coarse.Slope-10) > 1e-6 {
		t.Errorf("coarse slope: got %v want 10/sec", coarse.Slope)
	}
}
