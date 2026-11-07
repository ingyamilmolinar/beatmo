//go:build js && wasm && !test

package audio

import (
	"syscall/js"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Browser playback resolves per-instrument AudioBuffers, so the PCM must live in JS; a Go-side Sample would never be
// heard.
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

// The id stays a known instrument.
func UnregisterSamplePCM(id instrument.ID) {
	js.Global().Call("unregisterSamplePCM", string(id))
	// audio.js keeps no factory table: re-push the document's block.
	pushInstrumentDocByID(id)
}

// One CopyBytesToJS: per-element js.Value churn is the long-session OOM trap.
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
