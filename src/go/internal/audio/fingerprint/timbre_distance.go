package fingerprint

import "math"

// timbre_distance.go — TimbreDistance is a CLASSIFICATION distance (which
// instrument is this?), as opposed to Distance, which is tuned to give a smooth
// optimizer gradient. The difference matters: Distance normalizes ~11 terms to
// [0,1] and sums them near-equally, so two different instruments at the same
// pitch/loudness land at similar totals (everything clusters). TimbreDistance
// instead leans hard on the pitch/loudness/expression-INVARIANT identity cues —
// the harmonic-profile SHAPE (spectral envelope), MFCC, tristimulus, even/odd
// balance — and deliberately ignores raw-spectrum L2, vibrato, coupling and the
// amplitude envelope, which vary with the specific note and performance and blur
// instrument identity (a real violin's envelope ≠ a 2 s synth render's).

// TimbreWeights weights the identity sub-distances. Defaults emphasize the
// harmonic-profile shape + MFCC (the two strongest instrument-identity cues).
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

// DefaultTimbreWeights is the starting weight set (tuned empirically against the
// reference-attribution harness).
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

// TimbreDistanceWeighted is TimbreDistance with explicit weights (for the
// harness weight sweep). Symmetric; zero for identical fingerprints.
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

// partialShapeDist is the cosine distance (0=identical shape … 2=opposed) over
// the first-16 harmonic amplitudes, each vector L2-normalized so it compares the
// SHAPE of the harmonic series (the spectral envelope) independent of level or
// pitch — the single strongest instrument-identity cue.
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

// mfccL2 is the Euclidean distance over MFCC coefficients 1..12 (skip c0, an
// energy/level offset that is not a timbre cue).
func mfccL2(a, b [13]float64) float64 {
	s := 0.0
	for i := 1; i < 13; i++ {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s)
}

// formantShapeDist compares the two spectral envelopes' formant structure when
// both are available (the bridge-hill / formant peaks are a strong bore/body
// signature). Returns 0 when either is missing (no penalty). Uses the bridge
// hill freq + gain exposed on SpectralEnvelope.
func formantShapeDist(a, b *SpectralEnvelope) float64 {
	if a == nil || b == nil {
		return 0
	}
	fd := math.Abs(a.BridgeHillHz-b.BridgeHillHz) / 3000.0
	gd := math.Abs(a.BridgeHillGainDB-b.BridgeHillGainDB) / 20.0
	return clamp1(fd) + clamp1(gd)
}
