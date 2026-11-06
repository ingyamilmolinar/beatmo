//go:build js && wasm

package audio

import (
	"encoding/binary"
	"math"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// SetChannelEQ hands the channel's EQ program (BuildEQProgram — the same Go
// code desktop runs) to the beatmo-dsp worklet, which runs it through the same
// C (mix_dsp.c). An empty band list clears the EQ.
func SetChannelEQ(id channel.ID, sampleRate int, bands ...EQBand) {
	channelEQWrites.Add(1)
	if id.IsZero() {
		return // names no channel: never "" to JS, which reads it as the master bus
	}
	fn := js.Global().Get("setChannelEQ")
	if !fn.Truthy() {
		return
	}
	recordChannelEQ(id, eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)})
	if len(bands) == 0 {
		fn.Invoke(id.String(), js.Null())
		return
	}
	fn.Invoke(id.String(), float64sToJS(BuildEQProgram(sampleRate, bands)))
}

func ClearChannelProcessors(id channel.ID) {
	SetChannelEQ(id, 0)
}

func NewEQProcessor(sampleRate int, bands ...EQBand) Processor {
	return &passThrough{}
}

// float64sToJS copies xs into a new Float64Array.
func float64sToJS(xs []float64) js.Value {
	raw := make([]byte, 8*len(xs))
	for i, x := range xs {
		binary.LittleEndian.PutUint64(raw[8*i:], math.Float64bits(x))
	}
	f64 := js.Global().Get("Float64Array").New(len(xs))
	js.CopyBytesToJS(js.Global().Get("Uint8Array").New(f64.Get("buffer")), raw)
	return f64
}
