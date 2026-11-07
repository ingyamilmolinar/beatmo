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

// Recomputing a cmplx.Rect per butterfly was a large share of every analysis. The SAME expression the loop used, so the
// transform is bit-identical: the audio-tab digests compare raw bits.
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

// Bit-reversing every index on every transform was a fifth of the FFT; the same swaps in the same order keep it
// bit-identical.
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

// The single shared analysis FFT (NewFFTObserver builds on it). fftSize rounds UP to a power of two n: n/2+1 bins,
// binHz = sampleRate/n; zero-padded if shorter.
func MagnitudeSpectrum(w Wave, fftSize int, window WindowType) (mag []float64, binHz float64) {
	n := nextPow2(fftSize)
	sr := w.SampleRate
	if sr == 0 {
		sr = sr44100Default
	}
	// Each product is the one applyWindowInPlace forms, so the spectrum is bit-identical.
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

// The LAST size samples (zero-padded if shorter), Hann-windowed: Bins in dB (N/2+1, DC..Nyquist), FreqBins, BinHz,
// PeakFreq, PeakMag.
func NewFFTObserver(size int) Observer {
	return &fftObserver{size: nextPow2(size)}
}

func (o *fftObserver) Observe(w Wave) Observation {
	n := o.size
	sr := w.SampleRate
	if sr == 0 {
		sr = sr44100Default
	}

	// A trimmed Wave, because MagnitudeSpectrum reads from the start of what it is given.
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
