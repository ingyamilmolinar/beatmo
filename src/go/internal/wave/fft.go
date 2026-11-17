package wave

import (
	"math"
	"math/cmplx"
	"sync"
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
	for _, sw := range bitReversalSwaps(n) {
		buf[sw[0]], buf[sw[1]] = buf[sw[1]], buf[sw[0]]
	}

	// Butterfly stages.
	for size := 2; size <= n; size <<= 1 {
		half := size / 2
		tw := stageTwiddles(size)
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				even := buf[start+k]
				odd := buf[start+k+half] * tw[k]
				buf[start+k] = even + odd
				buf[start+k+half] = even - odd
			}
		}
	}
}

// twiddleCache holds each butterfly stage's twiddles (keyed by stage size),
// computed once. Recomputing one cmplx.Rect per butterfly made Sincos ~45 %
// of every spectral analysis. The values are produced by the SAME expression
// the loop used, so the transform is bit-identical
// (TestCachedFFTIsBitIdentical): the audio-tab digests compare raw bits.
var twiddleCache sync.Map // int -> []complex128

func stageTwiddles(size int) []complex128 {
	if v, ok := twiddleCache.Load(size); ok {
		return v.([]complex128)
	}
	half := size / 2
	wn := -2.0 * math.Pi / float64(size)
	tw := make([]complex128, half)
	for k := 0; k < half; k++ {
		tw[k] = cmplx.Rect(1, wn*float64(k))
	}
	v, _ := twiddleCache.LoadOrStore(size, tw)
	return v.([]complex128)
}

// swapCache holds each size's bit-reversal permutation as the (i, j > i)
// pairs it swaps, computed once: reversing every index's bits on every
// transform was a fifth of the FFT's time. Same swaps, same order, so the
// transform is unchanged bit for bit (TestCachedFFTIsBitIdentical).
var swapCache sync.Map // int -> [][2]int32

func bitReversalSwaps(n int) [][2]int32 {
	if v, ok := swapCache.Load(n); ok {
		return v.([][2]int32)
	}
	bits := 0
	for m := n; m > 1; m >>= 1 {
		bits++
	}
	var sw [][2]int32
	for i := 0; i < n; i++ {
		if j := bitReverse(i, bits); j > i {
			sw = append(sw, [2]int32{int32(i), int32(j)})
		}
	}
	v, _ := swapCache.LoadOrStore(n, sw)
	return v.([][2]int32)
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
	// Window straight into the transform buffer, from the start: truncates
	// if longer, zero-pads if shorter. Each product is the one
	// applyWindowInPlace forms, so the spectrum is unchanged bit for bit.
	buf := make([]complex128, n)
	coeffs := windowCoeffs(n, window)
	for i, s := range w.Samples[:min(n, len(w.Samples))] {
		buf[i] = complex(s*coeffs[i], 0)
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

// FFT is the in-place radix-2 transform behind MagnitudeSpectrum, exported for
// analysis that runs many frames and reuses one buffer (internal/soundview).
// len(buf) must be a power of two.
func FFT(buf []complex128) { fft(buf) }

// HannWindow returns the cached n-point Hann window MagnitudeSpectrum applies
// (shared and read-only: callers must not write to it).
func HannWindow(n int) []float64 { return windowCoeffs(n, WindowHann) }
