package fingerprint

import (
	"math"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// tremoloSine renders a decaying sine multiplied by (1 + depth·sin(2π·modHz·t)) —
// a controlled amplitude warble on top of the natural decay.
func tremoloSine(sr int, dur, f0, tau, modHz, depth float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		trem := 1 + depth*math.Sin(2*math.Pi*modHz*t)
		s[i] = trem * math.Exp(-t/tau) * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// sweepSine renders a linear frequency sweep from f0 to f1 over the signal — a
// spectrum that moves frame-to-frame (high spectral flux) at constant amplitude.
func sweepSine(sr int, dur, f0, f1 float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		// Instantaneous phase of a linear chirp: 2π(f0·t + (f1−f0)/(2·dur)·t²).
		phase := 2 * math.Pi * (f0*t + (f1-f0)/(2*dur)*t*t)
		s[i] = math.Sin(phase)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestWarble_TremoloRaisesDepth(t *testing.T) {
	const sr = 44100
	clean := KickAnalyze(expDecaySine(sr, 0.5, 200, 0.25))
	warbly := KickAnalyze(tremoloSine(sr, 0.5, 200, 0.25, 8, 0.5))

	if warbly.WarbleDepth <= clean.WarbleDepth {
		t.Errorf("tremolo WarbleDepth (%.4f) must exceed clean (%.4f)", warbly.WarbleDepth, clean.WarbleDepth)
	}
	// The 8 Hz tremolo should raise warble far above the clean decay's residual.
	if warbly.WarbleDepth <= 3*clean.WarbleDepth {
		t.Errorf("tremolo WarbleDepth (%.4f) should be >> clean (%.4f)", warbly.WarbleDepth, clean.WarbleDepth)
	}
	if warbly.WarbleDepth <= 0 {
		t.Errorf("tremolo WarbleDepth = %.4f, want > 0", warbly.WarbleDepth)
	}
}

func TestFlux_SweepRaisesFlux(t *testing.T) {
	const sr = 44100
	steady := KickAnalyze(expDecaySine(sr, 0.5, 200, 0.5))
	// Sweep 150 → 500 Hz (stays within the analyzed low/mid range, spectrum moves).
	swept := KickAnalyze(sweepSine(sr, 0.5, 150, 500))

	if swept.SpectralFluxAvg <= steady.SpectralFluxAvg {
		t.Errorf("swept SpectralFluxAvg (%.5f) must exceed steady (%.5f)", swept.SpectralFluxAvg, steady.SpectralFluxAvg)
	}
	if swept.SpectralFluxAvg <= 0 {
		t.Errorf("swept SpectralFluxAvg = %.5f, want > 0", swept.SpectralFluxAvg)
	}
}

func TestKickDistance_FluxTerm(t *testing.T) {
	const sr = 44100
	clean := KickAnalyze(expDecaySine(sr, 0.5, 200, 0.25))
	warbly := KickAnalyze(tremoloSine(sr, 0.5, 200, 0.25, 8, 0.5))

	d := KickDistance(clean, warbly)
	if d.Flux <= 0 {
		t.Errorf("KickDistance Flux term = %.5f, want clearly > 0", d.Flux)
	}
	// Identity + symmetry.
	if id := KickDistance(warbly, warbly); id.Flux != 0 {
		t.Errorf("KickDistance(x,x).Flux = %g, want 0", id.Flux)
	}
	if dr := KickDistance(warbly, clean); math.Abs(dr.Flux-d.Flux) > 1e-12 {
		t.Errorf("Flux term not symmetric: %g vs %g", d.Flux, dr.Flux)
	}
}

// --- optional real-WAV reference ---------------------------------------------

func TestKickAnalyze_SandyrbFluxInharmAttack(t *testing.T) {
	const path = "../../../../../36010__sandyrb__dnb-kick-003.wav"
	if _, err := os.Stat(path); err != nil {
		t.Skip("sandyrb reference WAV unavailable")
	}
	w, ok := decodeWAVMono(path)
	if !ok {
		t.Skip("sandyrb WAV decode failed")
	}
	fp := KickAnalyze(w)
	t.Logf("sandyrb: SpectralFluxAvg=%.5f WarbleDepth=%.5f", fp.SpectralFluxAvg, fp.WarbleDepth)
	t.Logf("sandyrb: Inharmonicity=%.5f", fp.Inharmonicity)
	t.Logf("sandyrb: LogAttackTime=%.4f TemporalCentroidSec=%.5f", fp.LogAttackTime, fp.TemporalCentroidSec)
}
