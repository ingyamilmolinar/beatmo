package wave

import "math"

// ampToDB converts a linear amplitude to decibels.
// Returns -math.MaxFloat64 for zero amplitude (negative infinity).
func ampToDB(amp float64) float64 {
	if amp == 0 {
		return -math.MaxFloat64
	}
	return 20 * math.Log10(amp)
}

// --- PeakRMS Observer ---

type peakRMSObserver struct{}

// NewPeakRMSObserver returns an Observer that computes peak level (dB),
// RMS level (dB), and clip count (samples exceeding +/-1.0).
func NewPeakRMSObserver() Observer {
	return &peakRMSObserver{}
}

func (o *peakRMSObserver) Observe(w Wave) Observation {
	n := len(w.Samples)
	if n == 0 {
		return Observation{
			Kind:   ObsPeakRMS,
			PeakDB: -math.MaxFloat64,
			RMSDB:  -math.MaxFloat64,
		}
	}

	var peak, sumSq float64
	var clips int
	for _, s := range w.Samples {
		a := math.Abs(s)
		if a > peak {
			peak = a
		}
		sumSq += s * s
		if a > 1.0 {
			clips++
		}
	}

	rms := math.Sqrt(sumSq / float64(n))

	return Observation{
		Kind:      ObsPeakRMS,
		Peak:      peak,
		RMS:       rms,
		PeakDB:    ampToDB(peak),
		RMSDB:     ampToDB(rms),
		ClipCount: clips,
	}
}

// --- Envelope Observer ---

type envelopeObserver struct {
	attackMs  float64
	releaseMs float64
}

// NewEnvelopeObserver returns an Observer that follows the amplitude
// envelope with configurable attack and release times in milliseconds.
// Attack controls how fast the envelope rises; release controls how
// fast it falls.
func NewEnvelopeObserver(attackMs, releaseMs float64) Observer {
	return &envelopeObserver{attackMs: attackMs, releaseMs: releaseMs}
}

func (o *envelopeObserver) Observe(w Wave) Observation {
	n := len(w.Samples)
	if n == 0 {
		return Observation{Kind: ObsEnvelope}
	}

	sr := float64(w.SampleRate)
	attackCoef := 1.0 - math.Exp(-1.0/(o.attackMs*0.001*sr))
	releaseCoef := 1.0 - math.Exp(-1.0/(o.releaseMs*0.001*sr))

	env := make([]float64, n)
	var current float64
	for i, s := range w.Samples {
		a := math.Abs(s)
		if a > current {
			current += attackCoef * (a - current)
		} else {
			current += releaseCoef * (a - current)
		}
		env[i] = current
	}

	return Observation{
		Kind:     ObsEnvelope,
		Envelope: env,
	}
}

// --- ZeroCrossing Observer ---

type zeroCrossingObserver struct{}

// NewZeroCrossingObserver returns an Observer that counts sign changes
// between consecutive samples and computes the crossing rate per second.
func NewZeroCrossingObserver() Observer {
	return &zeroCrossingObserver{}
}

func (o *zeroCrossingObserver) Observe(w Wave) Observation {
	n := len(w.Samples)
	if n < 2 {
		return Observation{Kind: ObsZeroCrossing}
	}

	crossings := 0
	for i := 1; i < n; i++ {
		if (w.Samples[i-1] > 0 && w.Samples[i] < 0) ||
			(w.Samples[i-1] < 0 && w.Samples[i] > 0) {
			crossings++
		}
	}

	dur := w.Duration()
	var rate float64
	if dur > 0 {
		rate = float64(crossings) / dur
	}

	return Observation{
		Kind:          ObsZeroCrossing,
		ZeroCrossings: crossings,
		ZCRate:        rate,
	}
}
