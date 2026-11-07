package fingerprint

import "math"

// A CLASSIFICATION distance (which instrument is this?), unlike Distance's smooth optimiser gradient where different
// instruments cluster. It leans on invariant identity cues (harmonic-profile shape, MFCC, tristimulus, even/odd) and
// ignores what varies with the note and performance (raw spectrum, vibrato, coupling, envelope).

// The harmonic-profile shape and MFCC are the strongest identity cues.
type TimbreWeights struct {
	Profile     float64 // harmonic-profile shape (cosine over normalized partials)
	MFCC        float64 // mel-cepstrum L2 (coeffs 1..12)
	Tristimulus float64 // fundamental/mid/high harmonic balance
	EvenOdd     float64 // cylindrical-vs-conical bore signature
	Centroid    float64 // brightness
	Inharmonic  float64 // metallic-vs-pure
	Flatness    float64 // tonal-vs-noisy
	Formant     float64 // spectral-envelope formant peaks (when available)
}

// Tuned against the reference-attribution harness.
var DefaultTimbreWeights = TimbreWeights{
	Profile:     3.0,
	MFCC:        2.0,
	Tristimulus: 1.0,
	EvenOdd:     0.5,
	Centroid:    0.5,
	Inharmonic:  0.5,
	Flatness:    0.5,
	Formant:     1.0,
}

// TimbreDistance returns the identity distance using DefaultTimbreWeights.
func TimbreDistance(ref, cand Fingerprint) float64 {
	return TimbreDistanceWeighted(ref, cand, DefaultTimbreWeights)
}

// Symmetric, zero for identical fingerprints.
func TimbreDistanceWeighted(ref, cand Fingerprint, w TimbreWeights) float64 {
	d := 0.0
	d += w.Profile * partialShapeDist(ref.Partials, cand.Partials)
	d += w.MFCC * (mfccL2(ref.MFCC, cand.MFCC) / 50.0)
	tri := 0.0
	for i := 0; i < 3; i++ {
		x := ref.Tristimulus[i] - cand.Tristimulus[i]
		tri += x * x
	}
	d += w.Tristimulus * math.Sqrt(tri)
	d += w.EvenOdd * math.Abs(ref.EvenOddRatio-cand.EvenOddRatio)
	d += w.Centroid * math.Abs(ref.SpectralCentroid-cand.SpectralCentroid) / 2000.0
	d += w.Inharmonic * math.Abs(ref.Inharmonicity-cand.Inharmonicity)
	d += w.Flatness * math.Abs(ref.SpectralFlatness-cand.SpectralFlatness)
	d += w.Formant * formantShapeDist(ref.SpectralEnv, cand.SpectralEnv)
	return d
}

// Cosine distance over L2-normalised harmonic amplitudes: the series' SHAPE, independent of level or pitch.
func partialShapeDist(a, b [16]float64) float64 {
	var na, nb, dot float64
	for i := 0; i < 16; i++ {
		na += a[i] * a[i]
		nb += b[i] * b[i]
		dot += a[i] * b[i]
	}
	if na <= 1e-12 || nb <= 1e-12 {
		return 1
	}
	cos := dot / (math.Sqrt(na) * math.Sqrt(nb))
	return 1 - cos
}

// c0 is a level offset, not a timbre cue.
func mfccL2(a, b [13]float64) float64 {
	s := 0.0
	for i := 1; i < 13; i++ {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s)
}

// The bridge hill and formants are a body signature; 0 (no penalty) when either envelope is missing.
func formantShapeDist(a, b *SpectralEnvelope) float64 {
	if a == nil || b == nil {
		return 0
	}
	fd := math.Abs(a.BridgeHillHz-b.BridgeHillHz) / 3000.0
	gd := math.Abs(a.BridgeHillGainDB-b.BridgeHillGainDB) / 20.0
	return clamp1(fd) + clamp1(gd)
}
