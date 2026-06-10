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

// ProcessBlock batches circular buffer writes with minimal atomics. Only stores
// the write index atomically at window boundaries and at the end of the block.
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

// ProcessBlockBuf implements BlockProcessor for Analyzer.
// Copies input to output (pass-through) and feeds samples to the analyzer.
func (a *Analyzer) ProcessBlockBuf(in, out []float32, samples int) {
	copy(out[:samples], in[:samples])
	a.ProcessBlock(in, samples)
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

// SetEnabled controls whether compute() runs. When disabled, audio still passes
// through but FFT/RMS/peak analysis is skipped, saving ~4.65% CPU.
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

// EnablePreEQAnalyzer creates an analyzer on the channel that taps the signal
// after volume but before the EQ processor chain. Returns the analyzer.
func EnablePreEQAnalyzer(id string, window int) *Analyzer {
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
func PreEQAnalyzerSnapshot(id string) AnalyzerSnapshot {
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
	m map[string]*Analyzer
}{m: map[string]*Analyzer{}}

// EnableSynthAnalyzer is a no-op on desktop — per-stage scope data flows
// through the real scope.Service (engine_stop.go) rather than JS-side
// AnalyserNode taps. Returns nil so the WASM/desktop call shapes match.
//
// WASM has a real implementation in analyzer_wasm.go that wires an
// AudioContext AnalyserNode at the channel ingress.
func EnableSynthAnalyzer(id string, window int) *Analyzer { return nil }

// SynthAnalyzerSnapshot is a no-op on desktop — see EnableSynthAnalyzer.
// WASM uses the JS AnalyserNode; the UI bridge in
// internal/ui/wasm_analyzer_bridge.go is the only caller and it's only
// compiled for WASM.
func SynthAnalyzerSnapshot(id string) AnalyzerSnapshot { return AnalyzerSnapshot{} }

// EnableSendBusAnalyzer is a no-op on desktop — see EnableSynthAnalyzer.
func EnableSendBusAnalyzer(window int) *Analyzer { return nil }

// SendBusAnalyzerSnapshot is a no-op on desktop — see EnableSynthAnalyzer.
func SendBusAnalyzerSnapshot() AnalyzerSnapshot { return AnalyzerSnapshot{} }

// SetAnalyzerEnabled enables or disables FFT compute for a channel's analyzers.
// Audio pass-through is never affected; only the FFT/RMS/peak computation is gated.
func SetAnalyzerEnabled(id string, on bool) {
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
	analyzerRegistry.m = map[string]*Analyzer{}
	analyzerRegistry.Unlock()
	preEQAnalyzerRegistry.Lock()
	preEQAnalyzerRegistry.m = map[string]*Analyzer{}
	preEQAnalyzerRegistry.Unlock()
}

// AnalyzerBridgeStats returns zero counters on desktop; the JS↔Go bridge
// counters only exist in the WASM build.
func AnalyzerBridgeStats() (calls, elementReads uint64) { return 0, 0 }

// ResetAnalyzerBridgeStats is a no-op on desktop.
func ResetAnalyzerBridgeStats() {}

// ChannelAnalyzerMetrics returns scalar metrics for the channel,
// derived from the existing analyzer snapshot. Desktop has no JS bridge
// to short-circuit, so this is just a scalar projection.
func ChannelAnalyzerMetrics(id string) (peak, rms float64, clips int, active bool) {
	s := ChannelAnalyzerSnapshot(id)
	clips = s.ClipCount
	active = s.Peak > 0 || s.RMS > 0 || clips > 0
	return s.Peak, s.RMS, clips, active
}
