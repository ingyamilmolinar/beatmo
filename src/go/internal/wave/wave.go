// Package wave provides a minimal audio waveform type and composable
// interfaces for test signal generation, transformation, and observation.
//
// The Wave struct is the universal currency type: every generator produces
// one, every transform consumes and returns one, every observer inspects one.
// This package has zero dependencies on the audio engine, UI, or platform.
package wave

import "math"

// Wave holds a mono audio waveform as float64 samples.
type Wave struct {
	Samples    []float64
	SampleRate int
	Label      string
}

// Len returns the number of samples.
func (w Wave) Len() int { return len(w.Samples) }

// Duration returns the waveform duration in seconds.
// Returns 0 for an empty wave or zero sample rate.
func (w Wave) Duration() float64 {
	if len(w.Samples) == 0 || w.SampleRate == 0 {
		return 0
	}
	return float64(len(w.Samples)) / float64(w.SampleRate)
}

// PeakSample returns the maximum absolute sample value.
func (w Wave) PeakSample() float64 {
	peak := 0.0
	for _, s := range w.Samples {
		if a := math.Abs(s); a > peak {
			peak = a
		}
	}
	return peak
}

// Slice extracts a time range [startMs, endMs) in milliseconds.
// Out-of-range bounds are clamped to the waveform extent.
func (w Wave) Slice(startMs, endMs float64) Wave {
	if w.SampleRate == 0 || len(w.Samples) == 0 {
		return Wave{SampleRate: w.SampleRate, Label: w.Label}
	}
	msToSample := float64(w.SampleRate) / 1000.0
	start := int(math.Round(startMs * msToSample))
	end := int(math.Round(endMs * msToSample))
	if start < 0 {
		start = 0
	}
	if end > len(w.Samples) {
		end = len(w.Samples)
	}
	if start >= end {
		return Wave{SampleRate: w.SampleRate, Label: w.Label}
	}
	out := make([]float64, end-start)
	copy(out, w.Samples[start:end])
	return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label}
}

// Clone returns a deep copy of the wave. Mutating the clone does not
// affect the original.
func (w Wave) Clone() Wave {
	out := make([]float64, len(w.Samples))
	copy(out, w.Samples)
	return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label}
}

// --- Interfaces ---

// Source generates a Wave (e.g. a test signal generator).
type Source interface {
	Render() Wave
}

// Transform consumes a Wave and returns a new one (e.g. gain, filter).
type Transform interface {
	Apply(Wave) Wave
}

// ObsKind identifies the type of observation.
type ObsKind int

const (
	ObsFFT          ObsKind = iota // Frequency-domain analysis.
	ObsPeakRMS                     // Peak and RMS levels.
	ObsEnvelope                    // Amplitude envelope.
	ObsZeroCrossing                // Zero-crossing rate.
)

// Observation holds measurements produced by an Observer.
type Observation struct {
	Kind ObsKind

	// Peak/RMS fields.
	Peak float64
	RMS  float64

	// FFT fields.
	Bins      []float64 // Magnitude bins.
	BinHz     float64   // Frequency resolution per bin.
	PeakFreq  float64   // Dominant frequency.
	PeakMag   float64   // Magnitude at dominant frequency.

	// Envelope fields.
	Envelope []float64 // Amplitude envelope samples.

	// Zero-crossing fields.
	ZeroCrossings int     // Total zero crossings.
	ZCRate        float64 // Crossings per second.
}

// Observer inspects a Wave and returns an Observation.
type Observer interface {
	Observe(Wave) Observation
}

// --- Functional adapters ---

// TransformFunc adapts a plain function to the Transform interface.
type TransformFunc func(Wave) Wave

func (f TransformFunc) Apply(w Wave) Wave { return f(w) }

// ObserverFunc adapts a plain function to the Observer interface.
type ObserverFunc func(Wave) Observation

func (f ObserverFunc) Observe(w Wave) Observation { return f(w) }

// --- WaveSource ---

// WaveSource wraps an existing Wave as a Source.
type WaveSource struct {
	W Wave
}

func (s *WaveSource) Render() Wave { return s.W }

// --- Compositors ---

// Chain renders a Source, then applies each Transform in order.
func Chain(src Source, transforms ...Transform) Wave {
	w := src.Render()
	for _, t := range transforms {
		w = t.Apply(w)
	}
	return w
}

// Observe runs each Observer on the wave and returns all Observations.
func Observe(w Wave, observers ...Observer) []Observation {
	out := make([]Observation, len(observers))
	for i, o := range observers {
		out[i] = o.Observe(w)
	}
	return out
}

// Pipeline renders a Source through Transforms, then runs Observers on
// the final wave. Returns the transformed wave and all observations.
func Pipeline(src Source, transforms []Transform, observers []Observer) (Wave, []Observation) {
	w := Chain(src, transforms...)
	obs := Observe(w, observers...)
	return w, obs
}
