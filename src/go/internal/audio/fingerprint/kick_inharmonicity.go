package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// kick_inharmonicity.go — INHARMONICITY (metallic/wooden vs pure axis).
//
// Motivation: band energies, centroid, flatness and roughness all describe WHERE
// the energy is and how rough the partial spacing sounds, but none of them
// directly formalizes how far the body's partials sit from an exact harmonic
// series of the fundamental. That mistuning is precisely what a synth's
// mode-detune control (mode_detune) drives, and it is the perceptual "metallic /
// bell-like" ↔ "pure / harmonic" axis: a struck metal shell has strongly
// inharmonic modes; a clean sub-kick is near-harmonic.
//
// Method: on the sustained BODY region (same 35–150 ms window as the roughness
// metric, so "body" means the same thing everywhere), peak-pick the strongest
// partials (frequency + amplitude, with parabolic sub-bin refinement — the SAME
// peakPickPartials helper the roughness model uses). With f0 = the fingerprint's
// settled pitch (or the strongest partial when the pitch is unavailable), each
// partial is assigned its nearest harmonic number n = round(f/f0), n ≥ 1, and its
// fractional mistuning |f − n·f0| / (n·f0) is accumulated AMPLITUDE-WEIGHTED and
// normalized by the total amplitude. A perfectly harmonic stack reads ~0; a
// stretched/inharmonic stack reads clearly positive; a single pure tone reads 0.

const (
	// Body region — reuse the roughness/segment boundaries so "body" is identical
	// across metrics. Short kicks fall back to the whole signal.
	kickInharmBodyStartSec = segAttackEndMs / 1000 // 35 ms
	kickInharmBodyEndSec   = segBodyEndMs / 1000   // 150 ms

	// Partials below this frequency are ignored (DC / rumble).
	kickInharmMinHz = 30.0

	// Peak-pick at most this many partials (by amplitude).
	kickInharmMaxPartials = 10

	// Inharmonicity distance is |Δ Inharmonicity| × this, to bring the small raw
	// range (~0–0.1) up to the same order as the other KickDistance sub-terms.
	kickInharmDistScale = 10.0
)

// computeInharmonicity fills fp.Inharmonicity from the amplitude-weighted mean
// mistuning of the body-region partials relative to the settled fundamental.
// Mirrors the computeX pattern in kick_metrics.go. Must run AFTER computePitch so
// fp.PitchSettleHz is available as the fundamental.
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

// inharmonicity returns the amplitude-weighted mean fractional mistuning of a set
// of partials relative to fundamental f0:
//
//	inharm = ( Σ_i a_i · |f_i − n_i·f0| / (n_i·f0) ) / ( Σ_i a_i ),   n_i = round(f_i/f0), n_i ≥ 1
//
// 0 = every partial lands on an exact harmonic; larger = more inharmonic/metallic.
// A single partial that IS the fundamental yields 0. f0 ≤ 0 or no partials → 0.
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
