package fingerprint

import "math"

// Listeners write "warm", "airy", "sounds like a wind instead of a string"; nothing measured those, and the nearest axis
// (bright.centroid in absolute Hz) was anti-correlated with the ear. Each function is one perceptual property, defined
// so the number moves the way the word does.

// The centroid in MULTIPLES OF THE FUNDAMENTAL: dividing by f0 makes it timbre, not pitch, so a cello an octave up
// reads the same warmth. 0 with no reliable fundamental.
func BrightnessRatio(fp Fingerprint) float64 {
	if fp.F0Hz <= 0 {
		return 0
	}
	return fp.SpectralCentroid / fp.F0Hz
}

// dB per octave of harmonic number: where BrightnessRatio says where the energy sits, this says how fast it falls, and
// only it sees a few loud high partials on a dark spectrum. log2 because real excitations roll off geometrically. NaN
// with fewer than three partials.
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

// Dividing a late value by a near-zero onset would mean nothing.
const noisePersistenceFloor = 1e-4

// Late-half over early-half noise fraction: wind-vs-string and airy-vs-dry are one question. A blown instrument's
// noise persists (~1 or more), a bowed or plucked string's belongs to the onset (well below 1); a whole-signal
// average cannot tell them apart. ok=false with too little onset noise or too few frames.
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

// Breath, bow hiss and cymbal wash live above it, fundamentals and low partials below, so the measurement is about
// noise CHARACTER, not pitch.
const airBandHz = 2000.0

// Whole-spectrum flatness is dominated by the strong low partials, so breathy and clean notes score alike; above
// airBandHz it asks what "airy" means. ok=false with no bins or energy there.
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
