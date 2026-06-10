//go:build js && wasm && !test

package audio

import (
	"syscall/js"
	"unsafe"
)

// RenderInstrumentOneShot captures the active instrument's rendered one-shot on
// the browser. The PCM is produced by the C synth inside the JS WebAudio layer,
// so capture goes through the captureInstrumentPCM JS bridge, which returns the
// rendered Float32Array (as a Uint8Array view) plus its sample rate, or a falsy
// value when no render is available yet.
func RenderInstrumentOneShot(id string) ([]float32, int) {
	return captureInstrumentPCMOpts(id, false)
}

// RenderInstrumentOneShotRaw captures ignoring any non-destructive sample-edit
// descriptor (the JS bridge skips its applySampleEdit step when raw is set).
// The Sampler editor loads through this so it shows the UN-edited source
// waveform and overlays the saved trim/pitch/gain itself.
func RenderInstrumentOneShotRaw(id string) ([]float32, int) {
	return captureInstrumentPCMOpts(id, true)
}

func captureInstrumentPCMOpts(id string, raw bool) ([]float32, int) {
	sr := SampleRate()
	fn := js.Global().Get("captureInstrumentPCM")
	if !fn.Truthy() {
		return nil, sr
	}
	res := fn.Invoke(id, raw)
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
