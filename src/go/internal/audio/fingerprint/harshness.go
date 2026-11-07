package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Ride candidates matching the reference on every energy statistic were still judged "harsh": sharpness (Zwicker
// weighting above ~3 kHz), roughness (dissonance over partial pairs) and impulsiveness (first hit over the body) measure it.

// Bark-band magnitude stands in for specific loudness (the full model needs a calibrated SPL); the g(z) weighting above
// Bark 16 makes it "piercing", not "bright". Only meaningful compared with another signal measured the same way.
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

// The kick body-region measurement generalised to any window.
func RoughnessOf(w wave.Wave, fftSize int, minHz float64, maxPartials int) float64 {
	if len(w.Samples) < 8 {
		return 0
	}
	mag, binHz := wave.MagnitudeSpectrum(w, fftSize, wave.WindowHann)
	return plompLeveltRoughness(peakPickPartials(mag, binHz, minHz, maxPartials))
}

// The peak envelope over the following body in dB, on the RMS envelope so it tracks loudness: a delicate hit reads
// low.
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
	// 100–400 ms after the peak: past the strike, inside the ring of anything that sustains.
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
