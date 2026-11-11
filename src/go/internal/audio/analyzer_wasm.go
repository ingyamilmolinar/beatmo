//go:build js && wasm

package audio

import (
	"sync/atomic"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Analyzer is a handle on one of a channel's JS analysers: which channel, and
// which tap on it.
type Analyzer struct {
	ch      channel.ID
	tap     analyzerTap
	enabled bool
}

// analyzerTap is where on a channel's chain an analyser listens.
type analyzerTap int

const (
	tapChannel analyzerTap = iota // the channel's output
	tapPreEQ                      // before the channel EQ
	tapSynth                      // the channel's ingress, before any FX
)

type AnalyzerSnapshot struct {
	RMS       float64
	Peak      float64
	ClipCount int // running count of samples >|1.0|; populated by JS bridge when present
	Spectrum  []float64
	Waveform  []float64
}

// Diagnostic counters for the per-frame snapshot bridge. Bumped inside
// ChannelAnalyzerSnapshot / PreEQAnalyzerSnapshot to expose how often we
// cross the JS↔Go boundary and how many JS array elements we read per
// snapshot. AnalyzerBridgeStats() is the read accessor; the JS
// analyzerBridgeStats() export reads through it.
var (
	bridgeSnapshotCalls        atomic.Uint64
	bridgeSnapshotElementReads atomic.Uint64
)

// AnalyzerBridgeStats returns running totals for analyzer-bridge calls
// and element reads. Used by tests + the diag JS export to detect
// per-frame allocation pressure.
func AnalyzerBridgeStats() (calls, elementReads uint64) {
	return bridgeSnapshotCalls.Load(), bridgeSnapshotElementReads.Load()
}

// ResetAnalyzerBridgeStats zeroes the counters. Tests call this between
// phases to bound the measurement window.
func ResetAnalyzerBridgeStats() {
	bridgeSnapshotCalls.Store(0)
	bridgeSnapshotElementReads.Store(0)
}

// SetEnabled controls whether the analyzer is active (WASM). Analysis is in JS.
func (a *Analyzer) SetEnabled(on bool) { a.enabled = on }

// Enabled returns whether the analyzer is active (WASM).
func (a *Analyzer) Enabled() bool { return a.enabled }

// EnableChannelAnalyzer creates (or reuses) the channel's AnalyserNode.
//
// `window` is the caller's TIME-domain ask. The browser gives a channel one
// AnalyserNode serving both domains, and between the two it is the SPECTRUM
// whose requirement binds — a display band holding no FFT bin renders as
// silence (analyzer.SpectrumFFTSize) — so the node is created at
// BridgeAnalyserFFTSize and the caller's window is passed on separately as
// the time-domain export cap. Callers therefore get the frequency
// resolution the tab needs without the longer waveform none of them wants.
func EnableChannelAnalyzer(id channel.ID, window int) *Analyzer {
	if id.IsZero() {
		return &Analyzer{} // names no channel: never "" to JS, which reads it as the master bus
	}
	fn := js.Global().Get("enableChannelAnalyzer")
	if fn.Truthy() {
		waveSamples := window
		if waveSamples <= 0 {
			waveSamples = BridgeWaveformSamples
		}
		fn.Invoke(id.String(), BridgeAnalyserFFTSize, waveSamples)
	}
	return &Analyzer{ch: id, tap: tapChannel, enabled: true}
}

func (a *Analyzer) ProcessSample(x float64) float64       { return x }
func (a *Analyzer) ProcessBlock(samples []float32, n int) {}

// ProcessBlockBuf implements BlockProcessor for Analyzer (WASM).
// Analysis is done in JS; this just copies input to output.
func (a *Analyzer) ProcessBlockBuf(in, out []float32, samples int) {
	copy(out[:samples], in[:samples])
}

func (a *Analyzer) Snapshot() AnalyzerSnapshot {
	if a == nil {
		return AnalyzerSnapshot{}
	}
	switch a.tap {
	case tapPreEQ:
		return PreEQAnalyzerSnapshot(a.ch)
	case tapSynth:
		return SynthAnalyzerSnapshot(a.ch)
	}
	return ChannelAnalyzerSnapshot(a.ch)
}

func ChannelAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot {
	if id.IsZero() {
		return AnalyzerSnapshot{} // names no channel: never "" to JS, which reads it as the master bus
	}
	return channelSnapCache.getOrFetch(id, snapshotCacheTTL, func() AnalyzerSnapshot {
		bridgeSnapshotCalls.Add(1)
		fn := js.Global().Get("channelAnalyzerSnapshot")
		if !fn.Truthy() {
			return AnalyzerSnapshot{}
		}
		var out AnalyzerSnapshot
		// Ask for the analyser's full bin count. JS groups bins LINEARLY
		// when asked for fewer, which spends the loss where the display
		// bands are narrowest; its implicit default of 64 put 375 Hz
		// between bins and left every band under that empty.
		val := fn.Invoke(id.String(), BridgeSpectrumBins)
		if !val.Truthy() {
			return out
		}
		if v := val.Get("rms"); v.Truthy() {
			out.RMS = v.Float()
		}
		if v := val.Get("peak"); v.Truthy() {
			out.Peak = v.Float()
		}
		out.Spectrum = readSnapshotF64(val, "spectrumU8", "spectrumLen", "spectrum")
		out.Waveform = readSnapshotF64(val, "waveU8", "waveLen", "wave")
		return out
	})
}

// ChannelAnalyzerMetrics returns ONLY the scalar metrics for a channel —
// peak, RMS, clip count, and an `active` flag derived from peak/RMS. It
// skips the per-element spectrum + waveform readback that
// ChannelAnalyzerSnapshot performs, eliminating ~1024 js.Value
// allocations per call. Used by the Meters tab where the renderer
// consumes only scalars (see render_meters.go:38).
func ChannelAnalyzerMetrics(id channel.ID) (peak, rms float64, clips int, active bool) {
	if id.IsZero() {
		return 0, 0, 0, false // names no channel: never "" to JS, which reads it as the master bus
	}
	bridgeSnapshotCalls.Add(1)
	fn := js.Global().Get("channelAnalyzerSnapshot")
	if !fn.Truthy() {
		return 0, 0, 0, false
	}
	// bins=0 is the scalar-only contract: JS skips the frequency read, the
	// downsample and the wave copy. Without it this "fast path" still made
	// the browser compute a full spectrum + waveform on every call and then
	// read two numbers off it.
	val := fn.Invoke(id.String(), 0)
	if !val.Truthy() {
		return 0, 0, 0, false
	}
	if v := val.Get("rms"); v.Truthy() {
		rms = v.Float()
	}
	if v := val.Get("peak"); v.Truthy() {
		peak = v.Float()
	}
	if v := val.Get("clipCount"); v.Truthy() && v.Type() == js.TypeNumber {
		clips = v.Int()
	}
	active = peak > 0 || rms > 0 || clips > 0
	return peak, rms, clips, active
}

// EnablePreEQAnalyzer creates a pre-EQ analyzer on the channel (WASM version).
func EnablePreEQAnalyzer(id channel.ID, window int) *Analyzer {
	if id.IsZero() {
		return &Analyzer{} // names no channel: never "" to JS, which reads it as the master bus
	}
	fn := js.Global().Get("enablePreEQAnalyzer")
	if fn.Truthy() {
		fn.Invoke(id.String(), window)
	}
	return &Analyzer{ch: id, tap: tapPreEQ}
}

// PreEQAnalyzerSnapshot returns the latest pre-EQ snapshot (WASM version).
func PreEQAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot {
	if id.IsZero() {
		return AnalyzerSnapshot{} // names no channel: never "" to JS, which reads it as the master bus
	}
	return preEQSnapCache.getOrFetch(id, snapshotCacheTTL, func() AnalyzerSnapshot {
		bridgeSnapshotCalls.Add(1)
		fn := js.Global().Get("preEQAnalyzerSnapshot")
		if !fn.Truthy() {
			return AnalyzerSnapshot{}
		}
		var out AnalyzerSnapshot
		val := fn.Invoke(id.String())
		if !val.Truthy() {
			return out
		}
		if v := val.Get("rms"); v.Truthy() {
			out.RMS = v.Float()
		}
		if v := val.Get("peak"); v.Truthy() {
			out.Peak = v.Float()
		}
		out.Spectrum = readSnapshotF64(val, "spectrumU8", "spectrumLen", "spectrum")
		out.Waveform = readSnapshotF64(val, "waveU8", "waveLen", "wave")
		return out
	})
}

// EnableSynthAnalyzer creates a pre-FX (channel ingress) analyser on the
// channel. The scope panel uses this for both StageSynth (pre-everything) and
// StageAntiPop — in WASM the per-source anti-pop GainNode envelope is already
// applied before the signal reaches the channel ingress, so tapping ingress
// is the correct representation for both stages.
func EnableSynthAnalyzer(id channel.ID, window int) *Analyzer {
	if id.IsZero() {
		return &Analyzer{} // names no channel: never "" to JS, which reads it as the master bus
	}
	fn := js.Global().Get("enableSynthAnalyzer")
	if fn.Truthy() {
		fn.Invoke(id.String(), window)
	}
	return &Analyzer{ch: id, tap: tapSynth}
}

// SynthAnalyzerSnapshot returns the latest pre-FX channel-ingress snapshot.
// Companion to EnableSynthAnalyzer. Returns a zero snapshot if the JS export
// is missing (e.g., older audio.js without the bridge).
func SynthAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot {
	if id.IsZero() {
		return AnalyzerSnapshot{} // names no channel: never "" to JS, which reads it as the master bus
	}
	return synthSnapCache.getOrFetch(id, snapshotCacheTTL, func() AnalyzerSnapshot {
		bridgeSnapshotCalls.Add(1)
		fn := js.Global().Get("synthAnalyzerSnapshot")
		if !fn.Truthy() {
			return AnalyzerSnapshot{}
		}
		var out AnalyzerSnapshot
		val := fn.Invoke(id.String())
		if !val.Truthy() {
			return out
		}
		if v := val.Get("rms"); v.Truthy() {
			out.RMS = v.Float()
		}
		if v := val.Get("peak"); v.Truthy() {
			out.Peak = v.Float()
		}
		out.Spectrum = readSnapshotF64(val, "spectrumU8", "spectrumLen", "spectrum")
		out.Waveform = readSnapshotF64(val, "waveU8", "waveLen", "wave")
		return out
	})
}

// SetAnalyzerEnabled is a no-op on WASM (analysis runs in JS).
func SetAnalyzerEnabled(id channel.ID, on bool) {}

func resetAnalyzers() {}

// Reset is a no-op on WASM — the analyzer buffers live in JS AnalyserNodes, not
// in the Go struct. See ResetAllChannelAnalyzers for the WASM clear path.
func (a *Analyzer) Reset() {}

// ResetAllChannelAnalyzers drops the cached snapshot frames so the next fetch
// re-pulls fresh data from JS, and asks the JS bridge to zero its AnalyserNode
// taps if it exposes a resetAllAnalyzers hook (guarded — older audio.js without
// the hook is a no-op). Called on circuit load so stopped tabs read flat.
func ResetAllChannelAnalyzers() {
	channelSnapCache.clear()
	preEQSnapCache.clear()
	synthSnapCache.clear()
	if fn := js.Global().Get("resetAllAnalyzers"); fn.Truthy() {
		fn.Invoke()
	}
}
