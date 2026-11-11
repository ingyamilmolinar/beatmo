package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// weightedPartials renders a sum of steady sines with per-partial amplitudes — so
// the FUNDAMENTAL can be made dominant (as in a real pitched body), which is what
// lets the pitch tracker lock onto f0 rather than a louder harmonic. Inharmonicity
// is measured relative to that settled f0, so the fundamental must dominate.
func weightedPartials(sr int, dur float64, freqs, amps []float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		v := 0.0
		for k, f := range freqs {
			v += amps[k] * math.Sin(2*math.Pi*f*t)
		}
		s[i] = v
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// --- direct-formula sanity ----------------------------------------------------

func TestInharmonicity_HarmonicVsStretched(t *testing.T) {
	// f0 = 200. A harmonic stack lands exactly on n·f0 → 0. A stretched stack sits
	// off the harmonics → clearly positive.
	harm := inharmonicity([]spectralPartial{
		{Freq: 200, Amp: 1}, {Freq: 400, Amp: 1}, {Freq: 600, Amp: 1}, {Freq: 800, Amp: 1},
	}, 200)
	stretched := inharmonicity([]spectralPartial{
		{Freq: 200, Amp: 1}, {Freq: 420, Amp: 1}, {Freq: 650, Amp: 1}, {Freq: 890, Amp: 1},
	}, 200)

	if harm > 1e-9 {
		t.Errorf("harmonic-stack inharmonicity = %.5f, want ~0", harm)
	}
	if stretched <= 0.02 {
		t.Errorf("stretched-stack inharmonicity = %.5f, want clearly positive", stretched)
	}
	if stretched <= harm {
		t.Errorf("stretched (%.5f) must exceed harmonic (%.5f)", stretched, harm)
	}
}

func TestInharmonicity_PureTone(t *testing.T) {
	pure := inharmonicity([]spectralPartial{{Freq: 200, Amp: 1}}, 200)
	if pure != 0 {
		t.Errorf("single-partial-at-f0 inharmonicity = %g, want exactly 0", pure)
	}
}

// --- full-pipeline sanity (FFT-resolvable content) ---------------------------

func TestKickAnalyze_InharmonicityPipeline(t *testing.T) {
	const sr = 44100
	// FFT-resolvable partials (binHz ≈ 10.8 Hz at 4096/44.1 kHz; all spacings > 100 Hz).
	// Fundamental-dominant amplitudes so the pitch tracker settles on f0 = 200 Hz.
	amps := []float64{1.0, 0.5, 0.33, 0.25}
	harmonic := KickAnalyze(weightedPartials(sr, 0.4, []float64{200, 400, 600, 800}, amps))
	inharm := KickAnalyze(weightedPartials(sr, 0.4, []float64{200, 420, 650, 890}, amps))

	if inharm.Inharmonicity <= harmonic.Inharmonicity {
		t.Errorf("inharmonic Inharmonicity (%.4f) must exceed harmonic (%.4f)", inharm.Inharmonicity, harmonic.Inharmonicity)
	}
	if inharm.Inharmonicity <= 0 {
		t.Errorf("inharmonic Inharmonicity = %.4f, want > 0", inharm.Inharmonicity)
	}
	if harmonic.Inharmonicity > 0.02 {
		t.Errorf("harmonic Inharmonicity = %.4f, want near 0 (< 0.02)", harmonic.Inharmonicity)
	}

	d := KickDistance(harmonic, inharm)
	if d.Inharmonicity <= 0 {
		t.Errorf("KickDistance Inharmonicity term = %.4f, want clearly > 0", d.Inharmonicity)
	}
	// Identity + symmetry.
	if id := KickDistance(inharm, inharm); id.Inharmonicity != 0 {
		t.Errorf("KickDistance(x,x).Inharmonicity = %g, want 0", id.Inharmonicity)
	}
	if dr := KickDistance(inharm, harmonic); math.Abs(dr.Inharmonicity-d.Inharmonicity) > 1e-12 {
		t.Errorf("Inharmonicity term not symmetric: %g vs %g", d.Inharmonicity, dr.Inharmonicity)
	}
}
