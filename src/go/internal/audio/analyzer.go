//go:build !js && !test

package audio

import (
	"math"
	"sync"
	"sync/atomic"
)

// Analyzer is a lightweight realtime meter + spectrum tap.
// Uses lock-free atomics for the hot path (ProcessSample) to reduce contention.
type Analyzer struct {
	mu       sync.Mutex // Only used during compute
	window   []float64
	spectrum []float64
	fftBuf   []complex128 // Pre-allocated FFT buffer

	// Atomic state for lock-free hot path
	write  atomic.Int32
	filled atomic.Bool
	rmsBits atomic.Uint64  // Store float64 bits atomically
	peakBits atomic.Uint64 // Store float64 bits atomically

	// Snapshot copy for lock-free reads
	specSnap    []float64
	specSnapIdx atomic.Int32 // 0 or 1 for double-buffering
}

// AnalyzerSnapshot exposes the latest computed metrics.
type AnalyzerSnapshot struct {
	RMS      float64
	Peak     float64
	Spectrum []float64
	Waveform []float64
}

// NewAnalyzer creates an analyzer with the requested window size. The size is
// rounded to the nearest power-of-two between 64 and 8192.
func NewAnalyzer(window int) *Analyzer {
	if window < 64 {
		window = 64
	}
	if window > 8192 {
		window = 8192
	}
	window = nearestPow2(window)
	return &Analyzer{
		window: make([]float64, window),
		// spectrum holds N/2 bins (positive frequencies).
		spectrum: make([]float64, window/2),
		// Pre-allocated FFT buffer to avoid per-compute allocations.
		fftBuf: make([]complex128, window),
		// Double-buffered spectrum snapshot for lock-free reads.
		specSnap: make([]float64, window/2),
	}
}

// ProcessSample taps the stream and forwards the input unchanged.
// Uses lock-free atomics for the hot path to reduce contention.
func (a *Analyzer) ProcessSample(x float64) float64 {
	n := len(a.window)
	// Atomic increment, get previous value
	idx := int(a.write.Add(1) - 1)
	if idx >= n {
		// Wrap around: reset to 0 and process at index 0
		a.write.Store(1) // Next call will use index 1
		idx = 0
	}
	a.window[idx] = x // Single writer, no lock needed

	if idx == n-1 {
		a.filled.Store(true)
		a.compute() // Lock only during compute
	}
	return x
}

// Snapshot returns the latest measurements.
// Uses atomic reads for RMS/Peak and a snapshot copy of spectrum for lock-free access.
func (a *Analyzer) Snapshot() AnalyzerSnapshot {
	rms := math.Float64frombits(a.rmsBits.Load())
	peak := math.Float64frombits(a.peakBits.Load())

	// Copy spectrum with brief lock to avoid partial reads during compute
	a.mu.Lock()
	spec := make([]float64, len(a.spectrum))
	copy(spec, a.spectrum)
	a.mu.Unlock()

	// Export waveform in chronological order (oldest -> newest).
	var wave []float64
	writeIdx := int(a.write.Load())
	filled := a.filled.Load()
	if filled {
		wave = make([]float64, len(a.window))
		copy(wave, a.window[writeIdx:])
		copy(wave[len(a.window)-writeIdx:], a.window[:writeIdx])
	} else {
		wave = make([]float64, writeIdx)
		copy(wave, a.window[:writeIdx])
	}
	return AnalyzerSnapshot{
		RMS:      rms,
		Peak:     peak,
		Spectrum: spec,
		Waveform: wave,
	}
}

// compute performs FFT analysis. Locks only during spectrum write.
func (a *Analyzer) compute() {
	n := len(a.window)

	// Use pre-allocated FFT buffer
	for i := 0; i < n; i++ {
		a.fftBuf[i] = complex(a.window[i], 0)
	}

	// Compute RMS and peak locally
	var sumSq float64
	var peak float64
	for i := 0; i < n; i++ {
		v := a.window[i]
		if v < 0 {
			if -v > peak {
				peak = -v
			}
		} else if v > peak {
			peak = v
		}
		sumSq += v * v
	}

	fft(a.fftBuf)

	// Update spectrum with lock
	a.mu.Lock()
	half := n / 2
	for i := 0; i < half; i++ {
		re := real(a.fftBuf[i])
		im := imag(a.fftBuf[i])
		a.spectrum[i] = math.Hypot(re, im) / float64(n)
	}
	a.mu.Unlock()

	// Store RMS and peak atomically
	rms := math.Sqrt(sumSq / float64(n))
	a.rmsBits.Store(math.Float64bits(rms))
	a.peakBits.Store(math.Float64bits(peak))
}

// nearestPow2 rounds v to the nearest power-of-two (preferring the larger on ties).
func nearestPow2(v int) int {
	p := 1
	for p < v {
		p <<= 1
	}
	// If previous power is closer, step back.
	if p>>1 != 0 && (p-v) > (v-(p>>1)) {
		return p >> 1
	}
	return p
}

// fft performs an in-place iterative Cooley–Tukey radix-2 FFT.
func fft(buf []complex128) {
	n := len(buf)
	if n == 0 || (n&(n-1)) != 0 {
		return
	}
	// Bit-reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j &= ^bit
		}
		j |= bit
		if i < j {
			buf[i], buf[j] = buf[j], buf[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		theta := -2 * math.Pi / float64(length)
		wlen := complex(math.Cos(theta), math.Sin(theta))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			half := length >> 1
			for j := 0; j < half; j++ {
				u := buf[i+j]
				v := w * buf[i+j+half]
				buf[i+j] = u + v
				buf[i+j+half] = u - v
				w *= wlen
			}
		}
	}
}

// ---- Channel attachment helpers ----

var analyzerRegistry = struct {
	sync.RWMutex
	m map[string]*Analyzer
}{m: map[string]*Analyzer{}}

// EnableChannelAnalyzer inserts an analyzer into a channel's processor chain
// and returns the analyzer instance for polling.
func EnableChannelAnalyzer(id string, window int) *Analyzer {
	an := NewAnalyzer(window)
	AddChannelProcessor(id, an)
	analyzerRegistry.Lock()
	analyzerRegistry.m[id] = an
	analyzerRegistry.Unlock()
	return an
}

// ChannelAnalyzerSnapshot returns the latest snapshot for the channel, if any.
func ChannelAnalyzerSnapshot(id string) AnalyzerSnapshot {
	analyzerRegistry.RLock()
	an := analyzerRegistry.m[id]
	analyzerRegistry.RUnlock()
	if an == nil {
		return AnalyzerSnapshot{}
	}
	return an.Snapshot()
}

func resetAnalyzers() {
	analyzerRegistry.Lock()
	analyzerRegistry.m = map[string]*Analyzer{}
	analyzerRegistry.Unlock()
}
