package wave

import (
	"math"
	"math/cmplx"
)

// nextPow2 returns the smallest power of 2 >= n.
func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// fft performs an in-place iterative Cooley-Tukey radix-2 FFT on buf.
// len(buf) must be a power of 2.
func fft(buf []complex128) {
	n := len(buf)
	if n <= 1 {
		return
	}

	// Bit-reversal permutation.
	bits := 0
	for m := n; m > 1; m >>= 1 {
		bits++
	}
	for i := 0; i < n; i++ {
		j := bitReverse(i, bits)
		if j > i {
			buf[i], buf[j] = buf[j], buf[i]
		}
	}

	// Butterfly stages.
	for size := 2; size <= n; size <<= 1 {
		half := size / 2
		wn := -2.0 * math.Pi / float64(size)
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				twiddle := cmplx.Rect(1, wn*float64(k))
				even := buf[start+k]
				odd := buf[start+k+half] * twiddle
				buf[start+k] = even + odd
				buf[start+k+half] = even - odd
			}
		}
	}
}

// bitReverse reverses the lowest `bits` bits of v.
func bitReverse(v, bits int) int {
	r := 0
	for i := 0; i < bits; i++ {
		r = (r << 1) | (v & 1)
		v >>= 1
	}
	return r
}

// MagnitudeSpectrum returns the linear magnitude spectrum (DC..Nyquist) of the
// first fftSize samples of w (zero-padded if shorter), after applying the given
// window. binHz is the per-bin frequency resolution.
//
// fftSize is rounded UP to the next power of two (n); the returned slice has
// n/2+1 bins and binHz = sampleRate/n. Pass a power-of-two fftSize (as the
// fingerprint library does: 16384, 2048) to get exactly fftSize/2+1 bins.
// This is the single shared FFT entry point for analysis; NewFFTObserver
// builds its dB output on top of it.
func MagnitudeSpectrum(w Wave, fftSize int, window WindowType) (mag []float64, binHz float64) {
	n := nextPow2(fftSize)
	sr := w.SampleRate
	if sr == 0 {
		sr = sr44100Default
	}
	seg := make([]float64, n)
	copy(seg, w.Samples) // from the start; truncates if longer, zero-pads if shorter
	applyWindowInPlace(seg, window)
	buf := make([]complex128, n)
	for i, s := range seg {
		buf[i] = complex(s, 0)
	}
	fft(buf)
	numBins := n/2 + 1
	mag = make([]float64, numBins)
	for i := 0; i < numBins; i++ {
		mag[i] = cmplx.Abs(buf[i])
	}
	return mag, float64(sr) / float64(n)
}

// --- FFT Observer ---

type fftObserver struct {
	size int // FFT window size (power of 2).
}

// NewFFTObserver returns an Observer that performs an FFT on the wave.
// size is the FFT window size (rounded up to the next power of 2).
// If the wave is longer than size, the last size samples are used.
// If shorter, the wave is zero-padded.
// A Hann window is applied before the FFT.
// The returned Observation has Kind=ObsFFT with:
//   - Bins: magnitude spectrum in dB (N/2+1 bins, from DC to Nyquist)
//   - FreqBins: Hz label for each bin
//   - BinHz: frequency resolution per bin
//   - PeakFreq: dominant frequency (Hz)
//   - PeakMag: magnitude at dominant frequency (dB)
func NewFFTObserver(size int) Observer {
	return &fftObserver{size: nextPow2(size)}
}

func (o *fftObserver) Observe(w Wave) Observation {
	n := o.size
	sr := w.SampleRate
	if sr == 0 {
		sr = sr44100Default
	}

	// Extract the last n samples (or use all if shorter), preserving the
	// "last N" contract.  Pass a trimmed Wave to MagnitudeSpectrum so it
	// reads from the start of the slice (which is our last-N window).
	var lastN []float64
	if len(w.Samples) >= n {
		lastN = w.Samples[len(w.Samples)-n:]
	} else {
		lastN = w.Samples
	}
	trimmed := Wave{Samples: lastN, SampleRate: sr}

	linMag, binHz := MagnitudeSpectrum(trimmed, n, WindowHann)

	// Convert linear magnitudes to dB (normalized by N, matching original).
	numBins := len(linMag)
	bins := make([]float64, numBins)
	freqBins := make([]float64, numBins)

	peakIdx := 0
	peakMag := -math.MaxFloat64

	for i, lm := range linMag {
		db := ampToDB(lm / float64(n))
		bins[i] = db
		freqBins[i] = float64(i) * binHz

		if db > peakMag {
			peakMag = db
			peakIdx = i
		}
	}

	return Observation{
		Kind:     ObsFFT,
		Bins:     bins,
		FreqBins: freqBins,
		BinHz:    binHz,
		PeakFreq: freqBins[peakIdx],
		PeakMag:  peakMag,
	}
}

// sr44100Default is the fallback sample rate when Wave.SampleRate is 0.
const sr44100Default = 44100
