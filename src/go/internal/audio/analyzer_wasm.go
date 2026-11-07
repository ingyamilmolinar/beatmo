//go:build js && wasm

package audio

import (
	"sync/atomic"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

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

// How often the JS↔Go boundary is crossed and how many elements each snapshot reads.
var (
	bridgeSnapshotCalls        atomic.Uint64
	bridgeSnapshotElementReads atomic.Uint64
)

// Exposes per-frame allocation pressure to tests and the diagnostic JS export.
func AnalyzerBridgeStats() (calls, elementReads uint64) {
	return bridgeSnapshotCalls.Load(), bridgeSnapshotElementReads.Load()
}

func ResetAnalyzerBridgeStats() {
	bridgeSnapshotCalls.Store(0)
	bridgeSnapshotElementReads.Store(0)
}

// SetEnabled controls whether the analyzer is active (WASM). Analysis is in JS.
func (a *Analyzer) SetEnabled(on bool) { a.enabled = on }

// Enabled returns whether the analyzer is active (WASM).
func (a *Analyzer) Enabled() bool { return a.enabled }

// window is the caller's TIME-domain ask. One AnalyserNode serves both domains and the spectrum's requirement binds
// (a band with no bin renders as silence), so the node is created at BridgeAnalyserFFTSize and window caps the
// time-domain export separately.
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

// Analysis happens in JS; this only copies input to output.
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
		// The full bin count: JS groups bins LINEARLY when asked for fewer, emptying the narrowest (bass) bands.
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

// Scalars only, skipping the spectrum and waveform readback, for callers like Meters that read nothing else.
func ChannelAnalyzerMetrics(id channel.ID) (peak, rms float64, clips int, active bool) {
	if id.IsZero() {
		return 0, 0, 0, false // names no channel: never "" to JS, which reads it as the master bus
	}
	bridgeSnapshotCalls.Add(1)
	fn := js.Global().Get("channelAnalyzerSnapshot")
	if !fn.Truthy() {
		return 0, 0, 0, false
	}
	// bins=0 is the scalar-only contract: JS skips the frequency read, downsample and wave copy.
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

// The browser applies each source's anti-pop envelope before channel ingress, so the ingress tap represents both
// StageSynth and StageAntiPop.
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

// A zero snapshot when the JS export is missing.
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

// The buffers live in JS AnalyserNodes; ResetAllChannelAnalyzers clears them.
func (a *Analyzer) Reset() {}

// Drops the cached frames and asks JS to zero its taps when it exposes resetAllAnalyzers.
func ResetAllChannelAnalyzers() {
	channelSnapCache.clear()
	preEQSnapCache.clear()
	synthSnapCache.clear()
	if fn := js.Global().Get("resetAllAnalyzers"); fn.Truthy() {
		fn.Invoke()
	}
}
