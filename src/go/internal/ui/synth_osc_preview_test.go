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

// TestSynthOscSample_PhysicalModelsAndVoiceAreNotSine guards the OSC plot
// against the drift that let every physical-model oscillator (7..11) and Voice
// (12) plot as a plain sine: synthOscSample carried its OWN copy of the shape
// table covering only 0..6, so anything above fell through to a sine while the
// engine played something else entirely. It now delegates to
// audio.PreviewOscShape — the single table — and this pins that every oscillator
// osc_type offers plots as itself.
//
// osc_type 4 (FM) is excluded: its shape is the modulation, covered by
// TestSynthOscSample_FMUsesModulation below.
func TestSynthOscSample_PhysicalModelsAndVoiceAreNotSine(t *testing.T) {
	const n = 128
	collect := func(osc int) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = synthOscSample(osc, float64(i)/float64(n), 1, 0)
		}
		return out
	}
	sine := collect(0)
	for osc := 1; osc <= 12; osc++ {
		if osc == 4 {
			continue
		}
		got := collect(osc)
		var d float64
		for i := range got {
			d += math.Abs(got[i] - sine[i])
		}
		if d < 1e-6 {
			t.Errorf("osc_type %d plots IDENTICAL to Sine — the OSC plot's shape table "+
				"has drifted from audio.PreviewOscShape again", osc)
		}
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
