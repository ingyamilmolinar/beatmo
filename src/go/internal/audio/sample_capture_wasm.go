//go:build js && wasm && !test

package audio

import (
	"syscall/js"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The PCM is rendered by the C synth in the JS layer: falsy when no render exists yet.
func RenderInstrumentOneShot(id instrument.ID) ([]float32, int) {
	return captureInstrumentPCMOpts(id, false)
}

// The JS bridge skips applySampleEdit when raw is set.
func RenderInstrumentOneShotRaw(id instrument.ID) ([]float32, int) {
	return captureInstrumentPCMOpts(id, true)
}

func captureInstrumentPCMOpts(id instrument.ID, raw bool) ([]float32, int) {
	sr := SampleRate()
	fn := js.Global().Get("captureInstrumentPCM")
	if !fn.Truthy() {
		return nil, sr
	}
	res := fn.Invoke(string(id), raw)
	if !res.Truthy() {
		return nil, sr
	}
	if v := res.Get("sr"); v.Truthy() {
		sr = v.Int()
	}
	lenVal := res.Get("length")
	u8 := res.Get("bytes")
	if !lenVal.Truthy() || !u8.Truthy() {
		return nil, sr
	}
	n := lenVal.Int()
	if n <= 0 {
		return nil, sr
	}
	scratch := make([]byte, n*4)
	js.CopyBytesToGo(scratch, u8)
	f32 := unsafe.Slice((*float32)(unsafe.Pointer(&scratch[0])), n)
	out := make([]float32, n)
	copy(out, f32)
	return out, sr
}
