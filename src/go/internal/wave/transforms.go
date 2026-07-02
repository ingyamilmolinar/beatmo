package wave

import "math"

// WindowType identifies a window function shape.
type WindowType int

const (
	WindowHann WindowType = iota
	WindowHamming
	WindowBlackman
)

// NormalizeTransform returns a Transform that peak-normalizes to targetDB
// (0 dB = peak amplitude 1.0). Silence remains silence.
func NormalizeTransform(targetDB float64) Transform {
	return TransformFunc(func(w Wave) Wave {
		peak := w.PeakSample()
		if peak == 0 {
			return w.Clone()
		}
		targetAmp := math.Pow(10, targetDB/20.0)
		gain := targetAmp / peak
		out := make([]float64, len(w.Samples))
		for i, s := range w.Samples {
			out[i] = s * gain
		}
		return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label}
	})
}

// TrimTransform returns a Transform that extracts the time range
// [startMs, endMs) in milliseconds. Delegates to Wave.Slice.
func TrimTransform(startMs, endMs float64) Transform {
	return TransformFunc(func(w Wave) Wave {
		return w.Slice(startMs, endMs)
	})
}

// applyWindowInPlace multiplies seg by the given window function in place.
// Uses the periodic (DFT-even) form — divisor is len(seg), not len(seg)-1 —
// which is the standard choice for spectral analysis. See also WindowTransform,
// which uses the symmetric (n-1) form for filter design; the two conventions are
// deliberately distinct, so the coefficient arms are not shared.
func applyWindowInPlace(seg []float64, wt WindowType) {
	n := len(seg)
	if n == 0 {
		return
	}
	fn := float64(n)
	for i := range seg {
		var coeff float64
		switch wt {
		case WindowHann:
			coeff = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/fn))
		case WindowHamming:
			coeff = 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/fn)
		case WindowBlackman:
			coeff = 0.42 - 0.5*math.Cos(2*math.Pi*float64(i)/fn) + 0.08*math.Cos(4*math.Pi*float64(i)/fn)
		}
		seg[i] *= coeff
	}
}

// WindowTransform returns a Transform that applies the given window function.
// Note: uses the symmetric (n-1 divisor) form, suitable for filter design.
// For FFT spectral analysis, prefer applyWindowInPlace (periodic/n form).
func WindowTransform(wt WindowType) Transform {
	return TransformFunc(func(w Wave) Wave {
		n := len(w.Samples)
		if n == 0 {
			return w.Clone()
		}
		out := make([]float64, n)
		nm1 := float64(n - 1)
		for i, s := range w.Samples {
			var coeff float64
			switch wt {
			case WindowHann:
				coeff = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/nm1))
			case WindowHamming:
				coeff = 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/nm1)
			case WindowBlackman:
				coeff = 0.42 - 0.5*math.Cos(2*math.Pi*float64(i)/nm1) + 0.08*math.Cos(4*math.Pi*float64(i)/nm1)
			}
			out[i] = s * coeff
		}
		return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label}
	})
}

// ResampleTransform returns a Transform that resamples to targetRate using
// linear interpolation. If the rate is already equal, it returns a clone.
func ResampleTransform(targetRate int) Transform {
	return TransformFunc(func(w Wave) Wave {
		if w.SampleRate == targetRate || w.SampleRate == 0 || len(w.Samples) == 0 {
			return w.Clone()
		}
		ratio := float64(targetRate) / float64(w.SampleRate)
		newLen := int(math.Round(float64(len(w.Samples)) * ratio))
		out := make([]float64, newLen)
		for i := range out {
			srcPos := float64(i) / ratio
			idx := int(srcPos)
			frac := srcPos - float64(idx)
			if idx+1 < len(w.Samples) {
				out[i] = w.Samples[idx]*(1-frac) + w.Samples[idx+1]*frac
			} else if idx < len(w.Samples) {
				out[i] = w.Samples[idx]
			}
		}
		return Wave{Samples: out, SampleRate: targetRate, Label: w.Label}
	})
}
