package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// How far the body's partials sit from an exact harmonic series: the metallic ↔ pure axis mode_detune drives.
// Amplitude-weighted fractional mistuning |f − n·f0| / (n·f0) of the body-region partials against the settled f0 (or
// the strongest partial); harmonic reads ~0.

const (
	// The same body as roughness and segments; short kicks use the whole signal.
	kickInharmBodyStartSec = segAttackEndMs / 1000 // 35 ms
	kickInharmBodyEndSec   = segBodyEndMs / 1000   // 150 ms

	// Partials below this frequency are ignored (DC / rumble).
	kickInharmMinHz = 30.0

	// Peak-pick at most this many partials (by amplitude).
	kickInharmMaxPartials = 10

	// Brings the small raw range to the order of the other KickDistance terms.
	kickInharmDistScale = 10.0
)

// Must run AFTER computePitch, which provides the fundamental.
func (fp *KickFingerprint) computeInharmonicity(samples []float64, sr int) {
	a := secToSamples(kickInharmBodyStartSec, sr)
	b := secToSamples(kickInharmBodyEndSec, sr)
	if b > len(samples) {
		b = len(samples)
	}
	if a >= b {
		a, b = 0, len(samples) // very short kick → whole signal
	}
	if b-a < 8 {
		return
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: samples[a:b], SampleRate: sr}, kickBandFFT, wave.WindowHann)
	partials := peakPickPartials(mag, binHz, kickInharmMinHz, kickInharmMaxPartials)

	f0 := fp.PitchSettleHz
	if f0 <= 0 && len(partials) > 0 {
		f0 = partials[0].Freq // partials are sorted strongest-first
	}
	fp.Inharmonicity = inharmonicity(partials, f0)
}

// n_i = round(f_i/f0) ≥ 1. 0 when every partial is on a harmonic, for a lone fundamental, or for f0 ≤ 0 / no
// partials.
func inharmonicity(partials []spectralPartial, f0 float64) float64 {
	if f0 <= 0 || len(partials) == 0 {
		return 0
	}
	var num, den float64
	for _, p := range partials {
		if p.Freq <= 0 || p.Amp <= 0 {
			continue
		}
		n := math.Round(p.Freq / f0)
		if n < 1 {
			n = 1
		}
		dev := math.Abs(p.Freq-n*f0) / (n * f0)
		num += p.Amp * dev
		den += p.Amp
	}
	if den <= 0 {
		return 0
	}
	return num / den
}
