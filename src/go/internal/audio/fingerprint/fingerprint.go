// Package fingerprint is pure-DSP instrument fingerprinting over internal/wave: timbral descriptors and a Distance
// between a synthesized tone and a reference. It imports only internal/wave and the stdlib, so it never cycles
// with internal/audio.
package fingerprint

import (
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

type Fingerprint struct {
	Label      string
	SampleRate int

	// Spectral fields (filled by computeSpectral).
	F0Hz float64
	// 0..1, how much of the signal was periodic times how tightly those frames agreed. F0Hz is ALWAYS populated (the
	// octave guard returns a number even for a hi-hat), so anything assuming a harmonic series must consult this first.
	F0Confidence     float64
	Partials         [16]float64
	SpectralCentroid float64
	SpectralRolloff  float64
	EvenOddRatio     float64
	Inharmonicity    float64
	NoiseRatio       float64
	// The string-vs-bar discriminator Inharmonicity cannot give: a tuned bar's overtone at ~4·f0 reads "in tune" to a
	// nearest-integer check, while SeriesDeviation sees it is not the series' 2nd harmonic. Zero when F0Hz <= 0.
	PartialRatioProfile PartialRatios
	// Stored so Distance compares equal-length spectra without recomputing the FFT.
	SustainMag []float64

	// Temporal fields (filled by computeTemporal).
	Temporal *TemporalFingerprint

	// nil for silent input or an undetected F0.
	Vibrato *VibratoFingerprint `json:"vibrato,omitempty"`

	// nil for silent input.
	Harmonics *HarmonicTrajectory `json:"harmonics,omitempty"`

	// nil for silent input.
	SpectralEnv *SpectralEnvelope `json:"spectral_env,omitempty"`

	// nil for silent input.
	Coupling *DynamicCoupling `json:"coupling,omitempty"`

	// nil for silent input.
	NoiseProf *NoiseProfile `json:"noise_profile,omitempty"`

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

// Skips the attack transient before spectral analysis.
const SustainSkipSec = 0.3

// Every analyser uses the same window: skip 0.3 s, analyse to 1.3 s.
const SustainLenSec = 1.3

// Silent input (peak < 1e-6) returns a zero Fingerprint at once, so no division downstream sees zero energy.
func FromWave(w wave.Wave, label string) Fingerprint {
	sr := w.SampleRate
	if w.PeakSample() < 1e-6 {
		return Fingerprint{Label: label, SampleRate: sr}
	}
	w = PeakNormalize(w)
	fp := Fingerprint{Label: label, SampleRate: w.SampleRate}
	f0 := DetectF0Detailed(w)
	fp.F0Hz, fp.F0Confidence = f0.Hz, f0.Confidence
	computeSpectral(w, &fp)
	fp.Temporal = computeTemporal(w, fp.F0Hz)
	fp.MFCC = computeMFCC(w, 13)
	computeShape(w, &fp)
	computeEnvelope(w, &fp)

	// The same sustain window as the spectral and temporal passes.
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
