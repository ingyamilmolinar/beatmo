//go:build js && wasm

package audio

import "syscall/js"

type Analyzer struct {
	id      string
	enabled bool
}

type AnalyzerSnapshot struct {
	RMS      float64
	Peak     float64
	Spectrum []float64
	Waveform []float64
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

func (a *Analyzer) ProcessSample(x float64) float64 { return x }
func (a *Analyzer) ProcessBlock(samples []float32, n int)  {}

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
	if v := val.Get("spectrum"); v.Truthy() && v.Length() > 0 {
		n := v.Length()
		out.Spectrum = make([]float64, n)
		for i := 0; i < n; i++ {
			out.Spectrum[i] = v.Index(i).Float()
		}
	}
	if v := val.Get("wave"); v.Truthy() && v.Length() > 0 {
		n := v.Length()
		out.Waveform = make([]float64, n)
		for i := 0; i < n; i++ {
			out.Waveform[i] = v.Index(i).Float()
		}
	}
	return out
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
	if v := val.Get("spectrum"); v.Truthy() && v.Length() > 0 {
		n := v.Length()
		out.Spectrum = make([]float64, n)
		for i := 0; i < n; i++ {
			out.Spectrum[i] = v.Index(i).Float()
		}
	}
	if v := val.Get("wave"); v.Truthy() && v.Length() > 0 {
		n := v.Length()
		out.Waveform = make([]float64, n)
		for i := 0; i < n; i++ {
			out.Waveform[i] = v.Index(i).Float()
		}
	}
	return out
}

// SetAnalyzerEnabled is a no-op on WASM (analysis runs in JS).
func SetAnalyzerEnabled(id string, on bool) {}

func resetAnalyzers() {}
