//go:build js && wasm

package audio

import "syscall/js"

type Analyzer struct {
	id string
}

type AnalyzerSnapshot struct {
	RMS      float64
	Peak     float64
	Spectrum []float64
	Waveform []float64
}

func EnableChannelAnalyzer(id string, window int) *Analyzer {
	fn := js.Global().Get("enableChannelAnalyzer")
	if fn.Truthy() {
		fn.Invoke(id, window)
	}
	return &Analyzer{id: id}
}

func (a *Analyzer) ProcessSample(x float64) float64 { return x }

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

func resetAnalyzers() {}
