package synthmatch

import "testing"

// No shipped spec uses the Discrete or Int branches yet; locked so a future discrete Param is safe.
func TestClampParam_ContinuousClamp(t *testing.T) {
	p := Param{Key: "filter_cutoff", Min: 200, Max: 8000, Step: 100}
	cases := []struct{ in, want float64 }{
		{100, 200},   // below min → min
		{9000, 8000}, // above max → max
		{1500, 1500}, // in range → unchanged
		{200, 200},   // exactly min
		{8000, 8000}, // exactly max
	}
	for _, c := range cases {
		if got := clampParam(p, c.in); got != c.want {
			t.Errorf("clampParam(continuous, %.0f) = %.0f, want %.0f", c.in, got, c.want)
		}
	}
}

func TestClampParam_DiscreteSnapsToNearest(t *testing.T) {
	// Discrete overrides Min/Max/Step, even outside the span.
	p := Param{Key: "filter_type", Min: 0, Max: 2, Discrete: []float64{0, 1, 2}}
	cases := []struct{ in, want float64 }{
		{-5, 0},  // far below → nearest (0)
		{0.4, 0}, // rounds toward 0
		{0.6, 1}, // rounds toward 1
		{1.9, 2}, // rounds toward 2
		{100, 2}, // far above → nearest (2)
	}
	for _, c := range cases {
		if got := clampParam(p, c.in); got != c.want {
			t.Errorf("clampParam(discrete, %.2f) = %.0f, want %.0f", c.in, got, c.want)
		}
	}
}

func TestClampParam_IntRounds(t *testing.T) {
	// Int rounds to the nearest integer after the Min/Max clamp.
	p := Param{Key: "gen1_source", Min: 0, Max: 4, Int: true}
	cases := []struct{ in, want float64 }{
		{1.4, 1},
		{1.6, 2},
		{2.5, 3}, // math.Round: half away from zero
		{-1, 0},  // clamp to min, then round
		{9, 4},   // clamp to max, then round
	}
	for _, c := range cases {
		if got := clampParam(p, c.in); got != c.want {
			t.Errorf("clampParam(int, %.2f) = %.0f, want %.0f", c.in, got, c.want)
		}
	}
}
