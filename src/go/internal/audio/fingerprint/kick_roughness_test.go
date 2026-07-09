package fingerprint

import (
	"math"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// --- direct-formula sanity ----------------------------------------------------
//
// The FFT framing (fftSize 4096 @ 44.1 kHz → binHz ≈ 10.8 Hz) cannot resolve two
// partials a semitone apart at 200 Hz (≈11.9 Hz ≈ 1 bin); they merge into one
// peak. So the classic semitone/octave/pure sanity cases are validated on the
// roughness FUNCTION directly with constructed partials (deterministic, no FFT),
// which is what actually encodes the Plomp–Levelt curve. The FFT pipeline is
// exercised separately below with FFT-resolvable content.

func TestRoughness_SemitoneVsOctaveVsPure(t *testing.T) {
	semitone := math.Pow(2, 1.0/12) // 200 → ~211.9 Hz, ~a quarter critical band apart

	pure := plompLeveltRoughness([]spectralPartial{{Freq: 200, Amp: 1}})
	semi := plompLeveltRoughness([]spectralPartial{{Freq: 200, Amp: 1}, {Freq: 200 * semitone, Amp: 1}})
	octave := plompLeveltRoughness([]spectralPartial{{Freq: 200, Amp: 1}, {Freq: 400, Amp: 1}})

	if pure != 0 {
		t.Errorf("pure single-partial roughness = %g, want exactly 0", pure)
	}
	// Two equal partials cap the product-normalized roughness at 0.25·d_max
	// (d_max ≈ 0.045 at the curve's peak), so ~0.039 near a quarter critical band
	// is the practical maximum here — far above the octave/pure cases.
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
	// Inharmonic cluster (close, unequal spacings) vs an octave-spaced harmonic
	// stack of the SAME partial count and amplitudes.
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

// staticPartials renders a sum of steady (non-decaying) sines — used to feed the
// body-region roughness a controlled partial set that survives the FFT grid.
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
	// FFT-resolvable spacings (≥ ~2.5 bins ≈ 27 Hz apart): an inharmonic cluster
	// vs a harmonic/octave stack over the same span.
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

// --- optional real-WAV reference ---------------------------------------------

func TestKickAnalyze_SandyrbRoughnessDecay(t *testing.T) {
	const path = "../../../../../36010__sandyrb__dnb-kick-003.wav"
	if _, err := os.Stat(path); err != nil {
		t.Skip("sandyrb reference WAV unavailable")
	}
	w, ok := decodeWAVMono(path)
	if !ok {
		t.Skip("sandyrb WAV decode failed")
	}
	fp := KickAnalyze(w)
	t.Logf("sandyrb: Roughness=%.5f", fp.Roughness)
	t.Logf("sandyrb: BandT60=%v", fp.BandT60)
	t.Logf("sandyrb: BandT60Floored=%v DecayHopSec=%.4f", fp.BandT60Floored, fp.DecayHopSec)
	if fp.Roughness < 0 {
		t.Errorf("Roughness = %.5f, want >= 0", fp.Roughness)
	}
	if len(fp.BandT60) != KickBandCount {
		t.Errorf("BandT60 len = %d, want %d", len(fp.BandT60), KickBandCount)
	}
}
