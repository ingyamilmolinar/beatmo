package wave

import (
	"math"
	"math/rand"
)

// Sine generates a sine wave starting at phase 0.
func Sine(freq, amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	for i := range samples {
		t := float64(i) / float64(sampleRate)
		samples[i] = amplitude * math.Sin(2*math.Pi*freq*t)
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "sine"}
}

// Silence generates a wave of all zeros.
func Silence(durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	return Wave{Samples: make([]float64, n), SampleRate: sampleRate, Label: "silence"}
}

// Square generates a band-unlimited square wave (values alternate between
// +amplitude and -amplitude at the given frequency).
func Square(freq, amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	period := float64(sampleRate) / freq
	for i := range samples {
		phase := math.Mod(float64(i), period) / period
		if phase < 0.5 {
			samples[i] = amplitude
		} else {
			samples[i] = -amplitude
		}
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "square"}
}

// Impulse generates a wave with a single non-zero sample at sampleIndex.
// If sampleIndex is out of range, the wave is all zeros.
func Impulse(amplitude float64, sampleIndex int, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	if sampleIndex >= 0 && sampleIndex < n {
		samples[sampleIndex] = amplitude
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "impulse"}
}

// DC generates a constant-value wave where every sample equals amplitude.
func DC(amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = amplitude
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "dc"}
}

// Noise generates deterministic pseudo-random noise. The same seed always
// produces the same output. Values are uniformly distributed in
// [-amplitude, +amplitude].
func Noise(seed int64, amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	rng := rand.New(rand.NewSource(seed))
	for i := range samples {
		// rng.Float64() returns [0, 1); scale to [-amp, +amp).
		samples[i] = amplitude * (2*rng.Float64() - 1)
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "noise"}
}
