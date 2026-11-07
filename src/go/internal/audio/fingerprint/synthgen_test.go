package fingerprint

import (
	"math"
	"math/rand"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// partials[k] is harmonic k+1; scaled to ≤ 1 peak, then halved for headroom.
func synthTone(sr int, dur, f0 float64, partials []float64) wave.Wave {
	n := int(dur * float64(sr))
	samples := make([]float64, n)
	for i := range samples {
		t := float64(i) / float64(sr)
		v := 0.0
		for k, amp := range partials {
			v += amp * math.Sin(2*math.Pi*f0*float64(k+1)*t)
		}
		samples[i] = 0.5 * v
	}
	return wave.Wave{Samples: samples, SampleRate: sr}
}

// vibCents is the ±peak deviation; phase is integrated per sample.
func synthVibratoTone(sr int, dur, f0 float64, partials []float64, vibRate, vibCents float64) wave.Wave {
	n := int(dur * float64(sr))
	samples := make([]float64, n)
	phase := 0.0
	twoPiSR := 2 * math.Pi / float64(sr)
	for i := range samples {
		t := float64(i) / float64(sr)
		// Instantaneous fundamental with vibrato.
		modF := f0 * math.Pow(2, vibCents*math.Sin(2*math.Pi*vibRate*t)/1200)
		// Accumulate phase for H1; harmonics are integer multiples.
		phase += twoPiSR * modF
		v := 0.0
		for k, amp := range partials {
			v += amp * math.Sin(float64(k+1)*phase)
		}
		samples[i] = 0.5 * v
	}
	return wave.Wave{Samples: samples, SampleRate: sr}
}

// H2 swells 0 → 1 → 0 under a steady H1.
func synthSwellH2(sr int, dur, f0 float64) wave.Wave {
	n := int(dur * float64(sr))
	samples := make([]float64, n)
	fn := float64(n)
	for i := range samples {
		t := float64(i) / float64(sr)
		// Triangular envelope peaking at the midpoint.
		env := 1.0 - math.Abs(2.0*float64(i)/fn-1.0)
		h1 := math.Sin(2 * math.Pi * f0 * t)
		h2 := env * math.Sin(2*math.Pi*2*f0*t)
		// Scale so peak stays within ±1 (worst case both at 1.0 → 2.0 → scale by 0.45).
		samples[i] = 0.45 * (h1 + h2)
	}
	return wave.Wave{Samples: samples, SampleRate: sr}
}

// ~10 partials at 1/k through one RBJ band-pass.
func synthFormantTone(sr int, dur, f0, formantHz, q float64) wave.Wave {
	// Build harmonic source first (~10 partials at 1/k).
	nPartials := 10
	partials := make([]float64, nPartials)
	for k := range partials {
		partials[k] = 1.0 / float64(k+1)
	}
	src := synthTone(sr, dur, f0, partials)

	// Constant 0 dB peak: H(s) = (s/Q) / (s² + s/Q + 1).
	omega := 2 * math.Pi * formantHz / float64(sr)
	sinOmega := math.Sin(omega)
	cosOmega := math.Cos(omega)
	alpha := sinOmega / (2 * q)

	b0 := alpha
	b1 := 0.0
	b2 := -alpha
	a0 := 1 + alpha
	a1 := -2 * cosOmega
	a2 := 1 - alpha

	// Normalize by a0.
	b0 /= a0
	b1 /= a0
	b2 /= a0
	a1 /= a0
	a2 /= a0

	// Apply biquad filter (direct form II transposed).
	out := make([]float64, len(src.Samples))
	var x1, x2, y1, y2 float64
	for i, x := range src.Samples {
		y := b0*x + b1*x1 + b2*x2 - a1*y1 - a2*y2
		out[i] = y
		x2, x1 = x1, x
		y2, y1 = y1, y
	}
	return wave.Wave{Samples: out, SampleRate: sr}
}

// The louder, the brighter.
func synthBrightLoudTone(sr int, dur, f0 float64) wave.Wave {
	nPartials := 8
	n := int(dur * float64(sr))
	fn := float64(n)
	samples := make([]float64, n)
	for i := range samples {
		t := float64(i) / float64(sr)
		gain := 1.0 - math.Abs(2.0*float64(i)/fn-1.0) // triangular 0→1→0
		v := 0.0
		for k := 0; k < nPartials; k++ {
			// Upper partials get a boost proportional to the loudness.
			baseAmp := 1.0 / float64(k+1)
			var amp float64
			if k == 0 {
				amp = baseAmp
			} else {
				// Scale upper partials by gain so louder => brighter.
				amp = baseAmp * gain
			}
			v += amp * math.Sin(2*math.Pi*f0*float64(k+1)*t)
		}
		samples[i] = gain * 0.3 * v
	}
	return wave.Wave{Samples: samples, SampleRate: sr}
}

// Seeded deterministically.
func synthNoisyTone(sr int, dur, f0 float64, partials []float64, noiseRMS float64) wave.Wave {
	tone := synthTone(sr, dur, f0, partials)
	rng := rand.New(rand.NewSource(1))
	// Uniform [-1,1] has RMS 1/√3.
	noiseScale := noiseRMS * math.Sqrt(3)
	for i := range tone.Samples {
		tone.Samples[i] += noiseScale * (2*rng.Float64() - 1)
	}
	return tone
}
