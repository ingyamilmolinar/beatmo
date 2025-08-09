//go:build js && wasm
// +build js,wasm

package audio

import "syscall/js"

// exposed from web/audio.js
var (
	play   js.Value
	ctx    js.Value
	reset  js.Value
	setBPM js.Value
)

func init() {
	global := js.Global()
	play = global.Get("play") // (id, when) exported by JS
	ctx = global.Get("audioCtx")
	reset = global.Get("resetAudio")
	setBPM = global.Get("setBPM")
}

// Now returns the AudioContext's currentTime.
func Now() float64 {
	if ctx.Truthy() {
		return ctx.Get("currentTime").Float()
	}
	return 0
}

func Play(id string, when ...float64) {
	// Lazily grab the `play` function if it wasn't available at init time.
	if !play.Truthy() {
		play = js.Global().Get("play")
	}
	if !play.Truthy() {
		js.Global().Get("console").Call(
			"warn",
			"[audio] play function missing; dropping sample. Did you import audio.js before starting the WASM runtime?",
			id,
		)
		return
	}
	go func() {
		if len(when) > 0 {
			play.Invoke(id, when[0])
		} else {
			play.Invoke(id)
		}
	}()
}

func Reset() {
	if !reset.Truthy() {
		reset = js.Global().Get("resetAudio")
	}
	if reset.Truthy() {
		reset.Invoke()
	}
}

func SetBPM(bpm int) {
	if !setBPM.Truthy() {
		setBPM = js.Global().Get("setBPM")
	}
	if setBPM.Truthy() {
		setBPM.Invoke(bpm)
	}
}
