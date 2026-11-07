package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// At ~10.8 Hz/bin two partials a semitone apart at 200 Hz merge into one peak, so the semitone / octave / pure cases
// test the roughness FUNCTION on constructed partials; the FFT path is tested on resolvable content below.

func TestRoughness_SemitoneVsOctaveVsPure(t *testing.T) {
	semitone := math.Pow(2, 1.0/12) // 200 → ~211.9 Hz, ~a quarter critical band apart

	pure := plompLeveltRoughness([]spectralPartial{{Freq: 200, Amp: 1}})
	semi := plompLeveltRoughness([]spectralPartial{{Freq: 200, Amp: 1}, {Freq: 200 * semitone, Amp: 1}})
	octave := plompLeveltRoughness([]spectralPartial{{Freq: 200, Amp: 1}, {Freq: 400, Amp: 1}})

	if pure != 0 {
		t.Errorf("pure single-partial roughness = %g, want exactly 0", pure)
	}
	// Two equal partials cap the normalised roughness at 0.25·d_max, so ~0.039 is the practical maximum.
	if semi <= 0.03 {
		t.Errorf("semitone-apart roughness = %.4f, want HIGH (> 0.03)", semi)
	}
	if octave >= 0.01 {
		t.Errorf("octave-apart roughness = %.4f, want LOW (< 0.01, consonant)", octave)
	}
	if semi <= octave {
		t.Errorf("semitone (%.4f) must be rougher than octave (%.4f)", semi, octave)
	}
}

func TestRoughness_InharmonicVsHarmonic(t *testing.T) {
	// The same partial count and amplitudes.
	inharm := plompLeveltRoughness([]spectralPartial{
		{Freq: 200, Amp: 1}, {Freq: 227, Amp: 1}, {Freq: 258, Amp: 1},
	})
	harm := plompLeveltRoughness([]spectralPartial{
		{Freq: 200, Amp: 1}, {Freq: 400, Amp: 1}, {Freq: 600, Amp: 1},
	})
	if inharm <= harm {
		t.Errorf("inharmonic cluster (%.4f) must be rougher than harmonic stack (%.4f)", inharm, harm)
	}
	if harm >= 0.02 {
		t.Errorf("harmonic stack roughness = %.4f, want LOW (< 0.02)", harm)
	}
}

func TestRoughness_ScaleInvariant(t *testing.T) {
	// Level-invariance: scaling every amplitude leaves roughness unchanged.
	base := []spectralPartial{{Freq: 200, Amp: 1}, {Freq: 227, Amp: 0.7}, {Freq: 258, Amp: 0.5}}
	scaled := []spectralPartial{{Freq: 200, Amp: 10}, {Freq: 227, Amp: 7}, {Freq: 258, Amp: 5}}
	r1 := plompLeveltRoughness(base)
	r2 := plompLeveltRoughness(scaled)
	if math.Abs(r1-r2) > 1e-12 {
		t.Errorf("roughness not scale-invariant: %.6g vs %.6g", r1, r2)
	}
}

// --- full-pipeline sanity (FFT-resolvable content) ---------------------------

// Steady sines, so the partial set survives the FFT grid.
func staticPartials(sr int, dur float64, freqs []float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		v := 0.0
		for _, f := range freqs {
			v += math.Sin(2 * math.Pi * f * t)
		}
		s[i] = v / float64(len(freqs))
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestKickAnalyze_RoughnessPipeline(t *testing.T) {
	const sr = 44100
	// At least ~2.5 bins apart.
	rough := KickAnalyze(staticPartials(sr, 0.4, []float64{200, 231, 264, 299}))
	smooth := KickAnalyze(staticPartials(sr, 0.4, []float64{200, 400, 800, 1600}))

	if rough.Roughness <= smooth.Roughness {
		t.Errorf("inharmonic Roughness (%.4f) must exceed harmonic (%.4f)", rough.Roughness, smooth.Roughness)
	}
	if rough.Roughness <= 0 {
		t.Errorf("inharmonic Roughness = %.4f, want > 0", rough.Roughness)
	}

	// Distance: a rough kick vs a smooth kick has a clearly positive Roughness term.
	d := KickDistance(rough, smooth)
	if d.Roughness <= 0 {
		t.Errorf("KickDistance Roughness term = %.4f, want clearly > 0", d.Roughness)
	}
	// Identity + symmetry of the Roughness term.
	if id := KickDistance(rough, rough); id.Roughness != 0 {
		t.Errorf("KickDistance(x,x).Roughness = %g, want 0", id.Roughness)
	}
	if dr := KickDistance(smooth, rough); math.Abs(dr.Roughness-d.Roughness) > 1e-12 {
		t.Errorf("Roughness term not symmetric: %g vs %g", d.Roughness, dr.Roughness)
	}
}
