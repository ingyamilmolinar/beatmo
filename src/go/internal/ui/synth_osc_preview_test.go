package ui

import (
	"math"
	"testing"
)

// synthOscSample is the pure waveform function the OSC preview plots. It must
// produce distinct shapes per oscillator type so the preview reflects the
// user's generator choice.
func TestSynthOscSample_ShapesDiffer(t *testing.T) {
	const n = 64
	collect := func(osc int) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = synthOscSample(osc, float64(i)/float64(n), 1, 0)
		}
		return out
	}
	sine := collect(0)
	saw := collect(1)
	square := collect(2)
	tri := collect(3)

	// All within [-1,1].
	for _, set := range [][]float64{sine, saw, square, tri} {
		for _, v := range set {
			if v < -1.001 || v > 1.001 {
				t.Fatalf("sample %v out of [-1,1]", v)
			}
		}
	}
	differ := func(a, b []float64, name string) {
		var d float64
		for i := range a {
			d += math.Abs(a[i] - b[i])
		}
		if d < 1e-6 {
			t.Errorf("%s: shapes identical", name)
		}
	}
	differ(sine, saw, "sine vs saw")
	differ(saw, square, "saw vs square")
	differ(square, tri, "square vs triangle")

	// Square is bipolar ±1.
	if math.Abs(math.Abs(square[8])-1) > 1e-9 {
		t.Errorf("square not ±1: %v", square[8])
	}
}

func TestSynthOscSample_FMUsesModulation(t *testing.T) {
	// With FM (osc 4) a non-zero modulation depth must bend the wave away
	// from a pure sine.
	var d float64
	for i := 0; i < 64; i++ {
		tt := float64(i) / 64
		d += math.Abs(synthOscSample(4, tt, 2, 4) - math.Sin(2*math.Pi*tt))
	}
	if d < 1e-3 {
		t.Errorf("FM oscillator with depth produced ~pure sine (d=%g)", d)
	}
}
