package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// harshness.go — the HARSH/AGGRESSIVE vs MELLOW/DELICATE axis.
//
// Motivation: a listener rejected four successive ride candidates as "too
// harsh and aggressive and strong", wanting "mellow, softer, warmer with a
// delicate first hit and delicate tail" — while those candidates matched the
// reference on spectral centroid, flatness, low-band share, mode density and
// every decay time. Every axis in use was an ENERGY or SHAPE statistic, and
// none of them contains the word "harsh". Fitting harder on them could not
// have converged, because the thing being judged was not being measured.
//
// Three descriptors, each for a distinct half of the complaint:
//
//   Sharpness    — "harsh / piercing / strong". Zwicker's weighting rises
//                  steeply above ~3 kHz, so it is NOT the spectral centroid:
//                  two sounds with the same centroid differ in sharpness when
//                  one puts its energy above 3 kHz and the other side of it.
//   Roughness    — "aggressive / buzzy". Plomp-Levelt/Sethares sensory
//                  dissonance over partial PAIRS. This is the one that a dense
//                  synthetic mode bank gets wrong: packing hundreds of modes
//                  into a band maximizes exactly the quarter-critical-band
//                  spacing the ear hears as rough, so "more modes" can measure
//                  denser and sound angrier.
//   Impulsiveness— "the first hit is too direct". How much the attack spikes
//                  above the body that follows it.
//
// References:
//   E. Zwicker & H. Fastl, "Psychoacoustics: Facts and Models" (3rd ed.,
//     Springer 2007), ch. 9 — sharpness in acum, the g(z) weighting.
//   Plomp & Levelt (1965); Sethares (1993, 2005) — see kick_roughness.go,
//     whose plompLeveltRoughness this reuses rather than restating.

// SharpnessAcum returns Zwicker-style sharpness for a magnitude spectrum.
//
// Specific loudness is approximated by the magnitude in each Bark band (the
// full Zwicker loudness model needs a calibrated SPL the corpus does not
// carry); the g(z) weighting above Bark 16 is what makes this "piercing" and
// not "bright". Returned in acum-like units — the ABSOLUTE value is only
// meaningful compared against another signal measured the same way, which is
// the only way it is used.
func SharpnessAcum(mag []float64, binHz float64) float64 {
	if len(mag) == 0 || binHz <= 0 {
		return 0
	}
	var num, den float64
	for i, m := range mag {
		f := float64(i) * binHz
		if f < 20 || f > 20000 {
			continue
		}
		z := barkOf(f)
		g := 1.0
		if z > 16 {
			// Zwicker's steep rise above Bark 16 (~3 kHz).
			g = 0.066 * math.Exp(0.171*z)
		}
		n := m // specific-loudness stand-in
		num += n * g * z
		den += n
	}
	if den <= 0 {
		return 0
	}
	return 0.11 * num / den
}

// barkOf converts Hz to the Bark scale (Traunmuller 1990).
func barkOf(f float64) float64 {
	z := (26.81*f)/(1960+f) - 0.53
	if z < 0 {
		return 0
	}
	return z
}

// RoughnessOf is the Plomp-Levelt/Sethares sensory dissonance of an arbitrary
// window, generalizing kick_roughness.go's body-region measurement so any
// family can be judged on it.
func RoughnessOf(w wave.Wave, fftSize int, minHz float64, maxPartials int) float64 {
	if len(w.Samples) < 8 {
		return 0
	}
	mag, binHz := wave.MagnitudeSpectrum(w, fftSize, wave.WindowHann)
	return plompLeveltRoughness(peakPickPartials(mag, binHz, minHz, maxPartials))
}

// Impulsiveness is the ratio of the peak envelope to the body that follows it,
// in dB — "how much the strike sticks out above the ring". A gentle, delicate
// hit reads LOW; a hard direct one reads high. Measured on the RMS envelope so
// it tracks perceived loudness rather than a single sample excursion.
func Impulsiveness(samples []float64, rate int) float64 {
	const win = 256
	if rate <= 0 || len(samples) < win*8 {
		return 0
	}
	var env []float64
	for i := 0; i+win <= len(samples); i += win {
		var e float64
		for j := i; j < i+win; j++ {
			e += samples[j] * samples[j]
		}
		env = append(env, math.Sqrt(e/win))
	}
	var peak float64
	pk := 0
	for i, e := range env {
		if e > peak {
			peak, pk = e, i
		}
	}
	if peak <= 0 {
		return 0
	}
	// Body = 100..400 ms after the peak: past the strike, still well inside the
	// ring for anything that sustains.
	a := pk + (rate/10)/win
	b := pk + (4*rate/10)/win
	if b > len(env) {
		b = len(env)
	}
	if a >= b {
		return 0
	}
	var sum float64
	for i := a; i < b; i++ {
		sum += env[i]
	}
	body := sum / float64(b-a)
	if body <= 0 {
		return 0
	}
	return 20 * math.Log10(peak/body)
}
