// Package fingerprint provides pure-DSP instrument fingerprinting built
// exclusively on internal/wave. It computes timbral descriptors (spectral,
// temporal, envelope, MFCC) from a Wave and exposes a Distance function for
// comparing synthesized tones against reference recordings. This package
// intentionally imports only internal/wave and Go stdlib to avoid import
// cycles with internal/audio.
package fingerprint

import (
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Fingerprint holds the computed timbral descriptors for a single audio
// segment. The fields here are the skeleton; later tasks add AttackTimeSec,
// DecaySlope, RMSEnvelope, and MFCC coefficients.
type Fingerprint struct {
	Label      string
	SampleRate int

	// Spectral fields (filled by computeSpectral).
	F0Hz             float64
	Partials         [16]float64
	SpectralCentroid float64
	SpectralRolloff  float64
	EvenOddRatio     float64
	Inharmonicity    float64
	NoiseRatio       float64
	// SustainMag is the linear magnitude spectrum of the sustain window at
	// fftSize=16384. Stored so Distance can compare equal-length spectra
	// between ref and cand without recomputing the FFT.
	SustainMag []float64

	// Temporal fields (filled by computeTemporal).
	Temporal *TemporalFingerprint

	// Vibrato holds vibrato analysis (rate, extent, jitter, AM depth, onset).
	// Nil when the input is silent (early-out) or F0 is undetected.
	Vibrato *VibratoFingerprint `json:"vibrato,omitempty"`

	// Harmonics holds per-harmonic amplitude trajectories over the sustain window.
	// Nil when the input is silent (early-out).
	Harmonics *HarmonicTrajectory `json:"harmonics,omitempty"`

	// SpectralEnv holds the smoothed spectral envelope and its formant peaks.
	// Nil when the input is silent (early-out).
	SpectralEnv *SpectralEnvelope `json:"spectral_env,omitempty"`

	// Coupling holds the dynamic coupling metrics (brightness vs loudness correlation).
	// Nil when the input is silent (early-out).
	Coupling *DynamicCoupling `json:"coupling,omitempty"`

	// NoiseProf holds harmonic/residual energy decomposition metrics.
	// Nil when the input is silent (early-out).
	NoiseProf *NoiseProfile `json:"noise_profile,omitempty"`

	// MFCC holds the first 13 Mel-Frequency Cepstral Coefficients (filled by
	// computeMFCC): perceptual timbre descriptor from the sustain window.
	MFCC [13]float64

	// Spectral-shape fields (filled by computeShape).
	SpectralFlatness float64
	SpectralCrest    float64
	SpectralSkewness float64
	SpectralKurtosis float64
	Tristimulus      [3]float64

	// Envelope/timbre-noise fields (filled by computeEnvelope).
	AttackTimeSec  float64
	LogAttackTime  float64
	DecaySlope     float64 // dB/sec (post-peak least-squares; negative = decaying)
	SustainLevel   float64 // 0..1 relative to peak
	ReleaseTimeSec float64
	HNR            float64 // harmonic-to-noise ratio (dB-ish); higher = more tonal
}

// SustainSkipSec is the shared segmentation policy constant: skip the attack
// transient (first 0.3 seconds) before spectral analysis.
const SustainSkipSec = 0.3

// SustainLenSec is the shared segmentation policy constant: analyze up to
// 1.0 seconds of sustained material after the SustainSkipSec attack skip.
// All five analyzers (spectral, shape, MFCC, pitch/DetectF0, envelope/HNR) use
// the identical window: skip 0.3 s, analyze up to 1.3 s (a 1.0 s window).
const SustainLenSec = 1.3

// FromWave builds a Fingerprint from a raw waveform. Peak-normalizes, detects
// F0, then fills spectral metrics. Tasks 5–8 append temporal/MFCC/shape/envelope.
// If the input is silent (peak < 1e-6 after attempted normalization), a zero
// Fingerprint is returned immediately to prevent downstream NaN from divisions
// by zero energy.
func FromWave(w wave.Wave, label string) Fingerprint {
	sr := w.SampleRate
	// Early-out for silence: if peak magnitude is negligible, skip all analysis
	// so that no downstream division ever sees zero energy.
	if w.PeakSample() < 1e-6 {
		return Fingerprint{Label: label, SampleRate: sr}
	}
	w = PeakNormalize(w)
	fp := Fingerprint{Label: label, SampleRate: w.SampleRate}
	fp.F0Hz = DetectF0(w)
	computeSpectral(w, &fp)
	fp.Temporal = computeTemporal(w, fp.F0Hz)
	fp.MFCC = computeMFCC(w, 13)
	computeShape(w, &fp)
	computeEnvelope(w, &fp)

	// Compute the five new metric sub-structs on the same sustain window used by
	// the spectral/temporal passes above.
	sw := sustainWindow(w)
	cfg := DefaultAnalysisConfig()
	vib := computeVibrato(sw, fp.F0Hz, cfg)
	fp.Vibrato = &vib
	htr := computeHarmonicTrajectory(sw, fp.F0Hz, cfg)
	fp.Harmonics = &htr
	env := computeSpectralEnvelope(sw, cfg)
	fp.SpectralEnv = &env
	cpl := computeCoupling(sw, cfg)
	cpl.MacroCentroidRMSCorr = WholeSignalCentroidRMSCorr(w, cfg)
	fp.Coupling = &cpl
	nz := computeNoiseProfile(sw, fp.F0Hz, cfg)
	fp.NoiseProf = &nz

	return fp
}
