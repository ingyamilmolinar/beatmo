//go:build js && wasm && !test

package audio

import (
	"syscall/js"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// RegisterSamplePCM hands a raw PCM buffer to the JS WebAudio layer (which
// builds an AudioBuffer keyed by id) and records the id as a known instrument.
// Browser playback resolves per-instrument AudioBuffers, so a Go-side Sample
// would never be heard — the PCM must live in JS.
func RegisterSamplePCM(id instrument.ID, pcm []float32, sr int) {
	bumpVoiceRev(id) // new PCM: the lane must replace this instrument's not-started notes
	engine := SampleRate()
	js.Global().Call("registerSamplePCM", string(id), f32ToJSBytes(pcmAtEngineRate(pcm, sr, engine)), engine)
	instrumentsMu.Lock()
	found := false
	for _, x := range instruments {
		if x == id {
			found = true
			break
		}
	}
	if !found {
		instruments = append(instruments, id)
	}
	instrumentsMu.Unlock()
}

// UnregisterSamplePCM tells the JS WebAudio layer to drop the AudioBuffer/PCM
// registered for id and restore the instrument's built-in synth render, so a
// factory Reset makes the synth audible again instead of the leftover chop. The
// id stays in the known-instruments list (it is still a valid instrument).
func UnregisterSamplePCM(id instrument.ID) {
	js.Global().Call("unregisterSamplePCM", string(id))
	// The synth voice is the instrument's document: re-push its block so
	// audio.js renders it again (it keeps no factory table of its own).
	pushInstrumentDocByID(id)
}

// f32ToJSBytes copies pcm into a JS Uint8Array holding its little-endian byte
// representation. The JS side reinterprets it as a Float32Array. A single
// CopyBytesToJS avoids per-element js.Value churn (the long-session OOM trap).
func f32ToJSBytes(pcm []float32) js.Value {
	n := len(pcm)
	u8 := js.Global().Get("Uint8Array").New(n * 4)
	if n == 0 {
		return u8
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&pcm[0])), n*4)
	js.CopyBytesToJS(u8, bytes)
	return u8
}
