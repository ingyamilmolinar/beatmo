package fingerprint

import "math"

// perceptual.go — the measurements behind the words listeners actually use.
//
// Verdict notes are written in perceptual vocabulary: "warm", "airy", "dry",
// "sounds like a wind instrument instead of a string". None of the existing
// axes measured any of those directly, so a complaint could not be traced to a
// number and a number could not be traced back to a complaint. Worse, the
// closest existing axis was actively misleading: `bright.centroid` compares
// absolute hertz, so a note played an octave higher measures "brighter" for
// reasons that have nothing to do with timbre -- validated against the ear it
// scored AUC 0.36, ANTI-correlated.
//
// Each function here is one perceptual property, defined so the number moves
// the way the word does.

// BrightnessRatio is the spectral centroid expressed in MULTIPLES OF THE
// FUNDAMENTAL rather than in hertz: "how far above its own pitch does this
// note's energy sit".
//
// This is the warm/cool axis. Dividing by f0 is what makes it a timbre
// measurement instead of a pitch measurement -- a cello at 220 Hz and the same
// cello at 440 Hz should read the same warmth, and in absolute hertz they
// cannot. Measured on the corpus, the synth cello reads 21.6 against the
// reference's 11.1 (twice as bright relative to its own pitch) for an
// instrument the listener asked to be "much fuller and warmer"; the synth oboe
// reads 14.4 against 6.9, and its note says "needs warmness".
//
// Returns 0 when there is no reliable fundamental to normalise by.
func BrightnessRatio(fp Fingerprint) float64 {
	if fp.F0Hz <= 0 {
		return 0
	}
	return fp.SpectralCentroid / fp.F0Hz
}

// HarmonicSlope is the OLS slope of partial level (dB) against harmonic number
// (log2), i.e. dB per OCTAVE OF HARMONIC NUMBER.
//
// The complement to BrightnessRatio: the centroid says where the energy sits,
// this says how fast it falls off. A steep negative slope is a dark, warm,
// fundamental-dominant tone; a shallow one is bright and rich. They disagree
// usefully -- a spectrum can be dark on average while carrying a few loud high
// partials, and only the slope sees that.
//
// log2 of harmonic number rather than linear, because harmonic amplitude rolls
// off geometrically for every physical excitation, so this is the axis on
// which real instruments are roughly straight lines.
//
// Returns NaN when fewer than three partials carry energy -- a slope through
// two points is not a measurement.
func HarmonicSlope(partials [16]float64) float64 {
	var xs, ys []float64
	for k := 0; k < len(partials); k++ {
		if partials[k] <= 1e-9 {
			continue
		}
		xs = append(xs, math.Log2(float64(k+1)))
		ys = append(ys, 20*math.Log10(partials[k]))
	}
	if len(xs) < 3 {
		return math.NaN()
	}
	n := float64(len(xs))
	var sx, sy, sxx, sxy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
		sxx += xs[i] * xs[i]
		sxy += xs[i] * ys[i]
	}
	den := n*sxx - sx*sx
	if math.Abs(den) < 1e-12 {
		return math.NaN()
	}
	return (n*sxy - sx*sy) / den
}

// noisePersistenceFloor is the early-half noise level below which the ratio is
// not computed: dividing a late value by a near-zero onset produces an
// arbitrarily large number that means nothing.
const noisePersistenceFloor = 1e-4

// NoisePersistence is the late-half mean noise fraction divided by the
// early-half mean: does the note's noise STAY, or does it belong to the onset?
//
// This is the wind-vs-string axis, and the airy-vs-dry axis, in one number --
// because acoustically they are the same question. A blown instrument is
// excited by turbulent air for as long as it sounds, so its noise persists
// (ratio near or above 1). A bowed or plucked string has a scratchy,
// noisy attack that settles into a clean harmonic tone, so its noise decays
// (ratio well below 1). A whole-signal noise average cannot tell these apart:
// both can carry the same TOTAL noise while distributing it completely
// differently.
//
// The corpus is unambiguous. Every shipped bowed string measured ~0.99 --
// constant noise from first sample to last -- against references at 0.04
// (cello), 0.19 (violin) and 0.25 (viola). The listener's verdict on two of
// them was "sounds like a wind instrument, like a flute instead of a string",
// and on the third "too airy".
//
// ok=false when there is too little onset noise to divide by, or too few
// frames to split.
func NoisePersistence(noiseTrack []float64) (ratio float64, ok bool) {
	if len(noiseTrack) < 4 {
		return 0, false
	}
	half := len(noiseTrack) / 2
	var early, late float64
	for i, v := range noiseTrack {
		if i < half {
			early += v
		} else {
			late += v
		}
	}
	early /= float64(half)
	late /= float64(len(noiseTrack) - half)
	if early < noisePersistenceFloor {
		return 0, false
	}
	return late / early, true
}

// airBandHz is where "air" starts. Breath, bow hiss and cymbal wash live above
// it; the fundamental and the first few partials of every instrument in the
// corpus live below, so restricting to this band keeps the measurement about
// noise CHARACTER rather than about pitch.
const airBandHz = 2000.0

// AirBandFlatness is the spectral flatness of the magnitude spectrum ABOVE
// airBandHz: geometric mean over arithmetic mean, 0 = tonal, 1 = white.
//
// Whole-spectrum flatness (noise.flatness) is dominated by the strong low
// partials every pitched instrument has, so a breathy note and a clean one can
// score alike. Restricting to the air band asks the narrower question the word
// "airy" actually means: is there broadband hiss sitting on top of the tone?
//
// ok=false when the spectrum carries no bins above the band edge or no energy
// in them.
func AirBandFlatness(mag []float64, binHz float64) (flatness float64, ok bool) {
	if len(mag) == 0 || binHz <= 0 {
		return 0, false
	}
	start := int(math.Ceil(airBandHz / binHz))
	if start >= len(mag) {
		return 0, false
	}
	var logSum, sum float64
	n := 0
	for i := start; i < len(mag); i++ {
		v := mag[i]
		if v < 1e-12 {
			v = 1e-12
		}
		logSum += math.Log(v)
		sum += v
		n++
	}
	if n == 0 || sum <= 0 {
		return 0, false
	}
	geo := math.Exp(logSum / float64(n))
	arith := sum / float64(n)
	if arith <= 0 {
		return 0, false
	}
	return geo / arith, true
}
