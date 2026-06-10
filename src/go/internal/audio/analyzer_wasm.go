//go:build js && wasm

package audio

import (
	"sync/atomic"
	"syscall/js"
)

type Analyzer struct {
	id      string
	enabled bool
}

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

func EnableChannelAnalyzer(id string, window int) *Analyzer {
	fn := js.Global().Get("enableChannelAnalyzer")
	if fn.Truthy() {
		fn.Invoke(id, window)
	}
	return &Analyzer{id: id, enabled: true}
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
	return ChannelAnalyzerSnapshot(a.id)
}

func ChannelAnalyzerSnapshot(id string) AnalyzerSnapshot {
	return channelSnapCache.getOrFetch(id, snapshotCacheTTL, func() AnalyzerSnapshot {
		bridgeSnapshotCalls.Add(1)
		fn := js.Global().Get("channelAnalyzerSnapshot")
		if !fn.Truthy() {
			return AnalyzerSnapshot{}
		}
		var out AnalyzerSnapshot
		val := fn.Invoke(id)
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
func ChannelAnalyzerMetrics(id string) (peak, rms float64, clips int, active bool) {
	bridgeSnapshotCalls.Add(1)
	fn := js.Global().Get("channelAnalyzerSnapshot")
	if !fn.Truthy() {
		return 0, 0, 0, false
	}
	val := fn.Invoke(id)
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
func EnablePreEQAnalyzer(id string, window int) *Analyzer {
	fn := js.Global().Get("enablePreEQAnalyzer")
	if fn.Truthy() {
		fn.Invoke(id, window)
	}
	return &Analyzer{id: "preEQ:" + id}
}

// PreEQAnalyzerSnapshot returns the latest pre-EQ snapshot (WASM version).
func PreEQAnalyzerSnapshot(id string) AnalyzerSnapshot {
	return preEQSnapCache.getOrFetch(id, snapshotCacheTTL, func() AnalyzerSnapshot {
		bridgeSnapshotCalls.Add(1)
		fn := js.Global().Get("preEQAnalyzerSnapshot")
		if !fn.Truthy() {
			return AnalyzerSnapshot{}
		}
		var out AnalyzerSnapshot
		val := fn.Invoke(id)
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
func EnableSynthAnalyzer(id string, window int) *Analyzer {
	fn := js.Global().Get("enableSynthAnalyzer")
	if fn.Truthy() {
		fn.Invoke(id, window)
	}
	return &Analyzer{id: "synth:" + id}
}

// SynthAnalyzerSnapshot returns the latest pre-FX channel-ingress snapshot.
// Companion to EnableSynthAnalyzer. Returns a zero snapshot if the JS export
// is missing (e.g., older audio.js without the bridge).
func SynthAnalyzerSnapshot(id string) AnalyzerSnapshot {
	return synthSnapCache.getOrFetch(id, snapshotCacheTTL, func() AnalyzerSnapshot {
		bridgeSnapshotCalls.Add(1)
		fn := js.Global().Get("synthAnalyzerSnapshot")
		if !fn.Truthy() {
			return AnalyzerSnapshot{}
		}
		var out AnalyzerSnapshot
		val := fn.Invoke(id)
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

// EnableSendBusAnalyzer creates an analyser that taps the summed send-bus
// returns (delay + reverb). The Chain panel feeds it to StageSends.
func EnableSendBusAnalyzer(window int) *Analyzer {
	fn := js.Global().Get("enableSendBusAnalyzer")
	if fn.Truthy() {
		fn.Invoke(window)
	}
	return &Analyzer{id: "sendBus"}
}

// SendBusAnalyzerSnapshot returns the latest send-bus (delay+reverb returns)
// snapshot. Companion to EnableSendBusAnalyzer.
func SendBusAnalyzerSnapshot() AnalyzerSnapshot {
	return sendBusSnapshotCachedOrFetch(func() AnalyzerSnapshot {
		bridgeSnapshotCalls.Add(1)
		fn := js.Global().Get("sendBusAnalyzerSnapshot")
		if !fn.Truthy() {
			return AnalyzerSnapshot{}
		}
		var out AnalyzerSnapshot
		val := fn.Invoke()
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
func SetAnalyzerEnabled(id string, on bool) {}

func resetAnalyzers() {}
