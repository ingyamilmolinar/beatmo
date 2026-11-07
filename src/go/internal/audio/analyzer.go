//go:build !js && !test

package audio

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Lock-free atomics on the hot path (ProcessSample) keep the audio thread uncontended.
type Analyzer struct {
	mu       sync.Mutex // Only used during compute
	window   []float64
	spectrum []float64
	fftBuf   []complex128 // Pre-allocated FFT buffer

	// Atomic state for lock-free hot path
	write    atomic.Int32
	filled   atomic.Bool
	rmsBits  atomic.Uint64 // Store float64 bits atomically
	peakBits atomic.Uint64 // Store float64 bits atomically
	enabled  atomic.Bool   // When false, compute() is skipped (saves FFT CPU)

	// Snapshot copy for lock-free reads
	specSnap []float64
}

// AnalyzerSnapshot exposes the latest computed metrics.
type AnalyzerSnapshot struct {
	RMS       float64
	Peak      float64
	ClipCount int // running count of samples >|1.0|; surfaced for meter-bridge clip indicators
	Spectrum  []float64
	Waveform  []float64
}

// The size is rounded to the nearest power of two in [64, 8192].
func NewAnalyzer(window int) *Analyzer {
	if window < 64 {
		window = 64
	}
	if window > 8192 {
		window = 8192
	}
	window = nearestPow2(window)
	a := &Analyzer{
		window: make([]float64, window),
		// spectrum holds N/2 bins (positive frequencies).
		spectrum: make([]float64, window/2),
		// Pre-allocated FFT buffer to avoid per-compute allocations.
		fftBuf: make([]complex128, window),
		// Double-buffered spectrum snapshot for lock-free reads.
		specSnap: make([]float64, window/2),
	}
	a.enabled.Store(true)
	return a
}

// Forwards the input unchanged.
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

// The write index is stored atomically only at window boundaries and at the block end.
func (a *Analyzer) ProcessBlock(samples []float32, n int) {
	ws := len(a.window)
	if ws == 0 || n <= 0 {
		return
	}
	idx := int(a.write.Load())
	for i := 0; i < n; i++ {
		if idx >= ws {
			idx = 0
		}
		a.window[idx] = float64(samples[i])
		idx++
		if idx == ws {
			a.filled.Store(true)
			a.write.Store(int32(idx))
			a.compute()
			idx = 0
		}
	}
	a.write.Store(int32(idx % ws))
}

func (a *Analyzer) ProcessBlockBuf(in, out []float32, samples int) {
	copy(out[:samples], in[:samples])
	a.ProcessBlock(in, samples)
}

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

// Instances are reused across circuit loads and compute() overwrites only when a window fills, so a stopped analyzer
// would show the previous circuit forever. Only safe while the audio thread is idle (import stops playback first).
func (a *Analyzer) Reset() {
	if a == nil {
		return
	}
	a.mu.Lock()
	for i := range a.window {
		a.window[i] = 0
	}
	for i := range a.spectrum {
		a.spectrum[i] = 0
	}
	for i := range a.specSnap {
		a.specSnap[i] = 0
	}
	a.mu.Unlock()
	a.write.Store(0)
	a.filled.Store(false)
	a.rmsBits.Store(0)
	a.peakBits.Store(0)
}

// Audio still passes through; only the analysis is skipped.
func (a *Analyzer) SetEnabled(on bool) { a.enabled.Store(on) }

// Enabled returns whether the analyzer's compute path is active.
func (a *Analyzer) Enabled() bool { return a.enabled.Load() }

// compute performs FFT analysis. Locks only during spectrum write.
func (a *Analyzer) compute() {
	if !a.enabled.Load() {
		return
	}
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
	m map[channel.ID]*Analyzer
}{m: map[channel.ID]*Analyzer{}}

// IDEMPOTENT: channel focus and every tab transition re-issue it, so it reuses the analyzer already wired at that
// window; a stacked analyzer is another FFT on the audio thread.
func EnableChannelAnalyzer(id channel.ID, window int) *Analyzer {
	an := wireChannelAnalyzer(id, window)
	analyzerRegistry.Lock()
	analyzerRegistry.m[id] = an
	analyzerRegistry.Unlock()
	return an
}

// ChannelAnalyzerSnapshot returns the latest snapshot for the channel, if any.
func ChannelAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot {
	analyzerRegistry.RLock()
	an := analyzerRegistry.m[id]
	analyzerRegistry.RUnlock()
	if an == nil {
		return AnalyzerSnapshot{}
	}
	return an.Snapshot()
}

// Taps after volume, before the EQ chain.
func EnablePreEQAnalyzer(id channel.ID, window int) *Analyzer {
	an := NewAnalyzer(window)
	ch := chanMgr.ensureChannel(id)
	ch.mu.Lock()
	ch.preEQAnalyzer = an
	ch.mu.Unlock()
	preEQAnalyzerRegistry.Lock()
	preEQAnalyzerRegistry.m[id] = an
	preEQAnalyzerRegistry.Unlock()
	return an
}

// PreEQAnalyzerSnapshot returns the latest pre-EQ snapshot for the channel.
func PreEQAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot {
	preEQAnalyzerRegistry.RLock()
	an := preEQAnalyzerRegistry.m[id]
	preEQAnalyzerRegistry.RUnlock()
	if an == nil {
		return AnalyzerSnapshot{}
	}
	return an.Snapshot()
}

var preEQAnalyzerRegistry = struct {
	sync.RWMutex
	m map[channel.ID]*Analyzer
}{m: map[channel.ID]*Analyzer{}}

// A no-op on desktop, where per-stage scope data comes from scope.Service; nil keeps the call shape of the WASM
// version.
func EnableSynthAnalyzer(id channel.ID, window int) *Analyzer { return nil }

// A no-op on desktop; only the WASM bridge calls it.
func SynthAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot { return AnalyzerSnapshot{} }

// Pass-through is never affected; only the analysis is gated.
func SetAnalyzerEnabled(id channel.ID, on bool) {
	analyzerRegistry.RLock()
	an := analyzerRegistry.m[id]
	analyzerRegistry.RUnlock()
	if an != nil {
		an.SetEnabled(on)
	}
	preEQAnalyzerRegistry.RLock()
	pre := preEQAnalyzerRegistry.m[id]
	preEQAnalyzerRegistry.RUnlock()
	if pre != nil {
		pre.SetEnabled(on)
	}
}

func resetAnalyzers() {
	analyzerRegistry.Lock()
	analyzerRegistry.m = map[channel.ID]*Analyzer{}
	analyzerRegistry.Unlock()
	preEQAnalyzerRegistry.Lock()
	preEQAnalyzerRegistry.m = map[channel.ID]*Analyzer{}
	preEQAnalyzerRegistry.Unlock()
}

// Unlike resetAnalyzers, keeps the registrations: the reused instances would otherwise hold stale frames.
func ResetAllChannelAnalyzers() {
	analyzerRegistry.RLock()
	for _, an := range analyzerRegistry.m {
		an.Reset()
	}
	analyzerRegistry.RUnlock()
	preEQAnalyzerRegistry.RLock()
	for _, an := range preEQAnalyzerRegistry.m {
		an.Reset()
	}
	preEQAnalyzerRegistry.RUnlock()
}

// The bridge counters exist only in the WASM build.
func AnalyzerBridgeStats() (calls, elementReads uint64) { return 0, 0 }

// ResetAnalyzerBridgeStats is a no-op on desktop.
func ResetAnalyzerBridgeStats() {}

// Desktop has no bridge to skip: a scalar projection of the snapshot.
func ChannelAnalyzerMetrics(id channel.ID) (peak, rms float64, clips int, active bool) {
	s := ChannelAnalyzerSnapshot(id)
	clips = s.ClipCount
	active = s.Peak > 0 || s.RMS > 0 || clips > 0
	return s.Peak, s.RMS, clips, active
}
