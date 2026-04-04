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

	// Extract the last n samples (or zero-pad if shorter).
	segment := make([]float64, n)
	if len(w.Samples) >= n {
		copy(segment, w.Samples[len(w.Samples)-n:])
	} else {
		// Copy what we have, rest stays zero.
		copy(segment, w.Samples)
	}

	// Apply Hann window.
	for i := 0; i < n; i++ {
		hann := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n)))
		segment[i] *= hann
	}

	// Build complex buffer and run FFT.
	buf := make([]complex128, n)
	for i, s := range segment {
		buf[i] = complex(s, 0)
	}
	fft(buf)

	// Compute magnitude spectrum (N/2 + 1 bins: DC to Nyquist).
	numBins := n/2 + 1
	binHz := float64(sr) / float64(n)
	bins := make([]float64, numBins)
	freqBins := make([]float64, numBins)

	peakIdx := 0
	peakMag := -math.MaxFloat64

	for i := 0; i < numBins; i++ {
		mag := cmplx.Abs(buf[i]) / float64(n)
		db := ampToDB(mag)
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
